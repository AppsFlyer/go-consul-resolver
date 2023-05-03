package consulresolver

import (
	"context"
	"fmt"
	"log"
	"math"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/AppsFlyer/go-consul-resolver/lb"
	"github.com/cenkalti/backoff/v4"
	"github.com/friendsofgo/errors"
	"github.com/hashicorp/consul/api"
	"go.uber.org/ratelimit"
)

// Balancer interface provides methods for selecting a target and updating its state
type Balancer interface {
	// Select returns a *api.ServiceEntry describing the selected target.
	// If Select failed to provide a viable target, it should return a non-nil error.
	// Important: Select must be non-blocking!
	Select() (*api.ServiceEntry, error)
	// UpdateTargets will be called periodically to refresh the Balancer's targets list from which the Balancer is allowed to select
	UpdateTargets(targets []*api.ServiceEntry)
}

// ServiceProvider provides a method for obtaining a list of *api.ServiceEntry entities from Consul
type ServiceProvider interface {
	ServiceMultipleTags(service string, tags []string, passingOnly bool, q *api.QueryOptions) ([]*api.ServiceEntry, *api.QueryMeta, error)
}

type ServiceResolver struct {
	log                  LogFn
	ctx                  context.Context
	client               ServiceProvider
	queryOpts            *api.QueryOptions
	balancer             Balancer
	spec                 ServiceSpec
	prioritizedInstances [][]*api.ServiceEntry
	mu                   sync.Mutex
	init                 chan struct{}
	initDone             sync.Once
}

// NewConsulResolver creates a new Consul Resolver
// ctx - a context used for graceful termination of the consul-watcher go routine.
// Note that canceling the context will render the resolver stale, and any attempt to use it will immediately return an error
// conf - the resolver's config
func NewConsulResolver(ctx context.Context, conf ResolverConfig) (*ServiceResolver, error) {

	if conf.Client == nil {
		return nil, errors.New("consul client must not be nil")
	}

	if conf.ServiceSpec.ServiceName == "" {
		return nil, errors.New("service name must not be empty")
	}

	if conf.Query == nil {
		conf.Query = &api.QueryOptions{}
	} else {
		conf.Query.WaitIndex = 0
	}

	if conf.Balancer == nil {
		conf.Balancer = &lb.RoundRobinLoadBalancer{}
	}

	if conf.Log == nil {
		conf.Log = log.Printf
	}

	datacenters := append([]string{""}, conf.FallbackDatacenters...)

	resolver := &ServiceResolver{
		log:                  conf.Log,
		ctx:                  ctx,
		queryOpts:            conf.Query,
		spec:                 conf.ServiceSpec,
		client:               conf.Client.Health(),
		balancer:             conf.Balancer,
		prioritizedInstances: make([][]*api.ServiceEntry, len(datacenters)),
		init:                 make(chan struct{}),
		initDone:             sync.Once{},
	}

	// Always prepend the local datacenter with the highest priority
	for priority, dc := range datacenters {
		go resolver.populateFromConsul(dc, priority)
	}

	return resolver, nil
}

// ServiceName returns the service name that the resolver is looking up
func (r *ServiceResolver) ServiceName() string {
	return r.spec.ServiceName
}

// Resolve returns a single ServiceAddress instance of the resolved target
func (r *ServiceResolver) Resolve(ctx context.Context) (ServiceAddress, error) {

	// make sure balancer initialized
	select {
	case <-ctx.Done():
		return ServiceAddress{}, ctx.Err()
	case <-r.ctx.Done():
		return ServiceAddress{}, r.ctx.Err()
	case <-r.init:
		break
	}

	t, err := r.balancer.Select()
	if err != nil {
		return ServiceAddress{}, errors.Wrap(err, fmt.Sprintf("failed to resolve address for service %s", r.spec.ServiceName))
	}
	var host string
	var port int

	// fallback to node address if Service.Address is empty
	if t.Service.Address != "" {
		host = t.Service.Address
	} else {
		host = t.Node.Address
	}

	// Override the discovered service port, if needed
	if r.spec.ServicePort > 0 {
		port = r.spec.ServicePort
	} else {
		port = t.Service.Port
	}

	return ServiceAddress{Host: host, Port: port}, nil
}

func (r *ServiceResolver) populateFromConsul(dcName string, dcPriority int) {
	rl := ratelimit.New(1) // limit consul queries to 1 per second
	bck := backoff.NewExponentialBackOff()
	bck.MaxElapsedTime = 0
	bck.MaxInterval = time.Second * 30

	q := *r.queryOpts

	q.WaitIndex = 0
	q.Datacenter = dcName
	for r.ctx.Err() == nil {
		rl.Take()
		err := backoff.RetryNotify(
			func() error {
				se, meta, err := r.client.ServiceMultipleTags(
					r.spec.ServiceName,
					r.spec.Tags,
					!r.spec.IncludeUnhealthy,
					&q,
				)
				if err != nil {
					return err
				}
				if meta.LastIndex < q.WaitIndex {
					q.WaitIndex = 0
				} else {
					q.WaitIndex = uint64(math.Max(float64(1), float64(meta.LastIndex)))
				}

				if targets, shouldUpdate := r.getTargetsForUpdate(se, dcPriority); shouldUpdate {
					r.balancer.UpdateTargets(targets)
				}

				r.initDone.Do(func() {
					close(r.init)
				})
				return nil
			},
			bck,
			func(err error, duration time.Duration) {
				r.log("[Consul Resolver] failure querying consul, sleeping %s - %s", duration, err.Error())
			},
		)
		if err != nil {
			r.log("[Consul Resolver] failure querying consul - %s", err.Error())
		}
	}
	r.log("[Consul Resolver] context canceled, stopping consul watcher")
}

// getTargetsForUpdate will update the LB only if:
// - The DC has healthy nodes
// - No DC with higher priority has healthy nodes
func (r *ServiceResolver) getTargetsForUpdate(se []*api.ServiceEntry, priority int) (res []*api.ServiceEntry, shouldUpdate bool) {
	sort.SliceStable(se, func(i, j int) bool {
		return se[i].Node.ID < se[j].Node.ID
	})
	// check if the target list is unchanged
	if reflect.DeepEqual(se, r.prioritizedInstances[priority]) {
		return nil, false
	}

	var found bool
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prioritizedInstances[priority] = se
	for i := 0; i <= len(r.prioritizedInstances)-1; i++ {
		if len(r.prioritizedInstances[i]) == 0 {
			continue
		}
		found = true
		if priority > i {
			break
		}
		res = r.prioritizedInstances[i]
		shouldUpdate = true
		return
	}

	// If no DC has any nodes, return an empty slice and signal the caller that an update is needed
	if !found {
		shouldUpdate = true
	}

	return
}
