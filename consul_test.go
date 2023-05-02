package consulresolver

import (
	"context"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/AppsFlyer/go-consul-resolver/lb"
	"github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/assert"
)

type MockClient struct {
	services []*api.ServiceEntry
}

func (c *MockClient) ServiceMultipleTags(_ string, _ []string, _ bool, q *api.QueryOptions) (
	[]*api.ServiceEntry, *api.QueryMeta, error) {

	var opts *api.QueryOptions
	if q != nil {
		opts = q
	} else {
		opts = &api.QueryOptions{}
	}

	if opts.WaitIndex != 0 {
		time.Sleep(2 * time.Second)
	}

	return c.services,
		&api.QueryMeta{LastIndex: opts.WaitIndex + 1},
		nil
}

func TestConsulResolver(t *testing.T) {

	serviceName := "service"

	endpoints := []*api.ServiceEntry{
		{
			Node:    &api.Node{},
			Service: &api.AgentService{Address: "localhost", Service: serviceName, Port: 8080},
			Checks:  api.HealthChecks{},
		},
		{
			Node:    &api.Node{},
			Service: &api.AgentService{Address: "localhost2", Service: serviceName, Port: 8081},
			Checks:  api.HealthChecks{},
		},
	}

	c := &MockClient{endpoints}
	r := &ServiceResolver{
		client:               c,
		ctx:                  context.Background(),
		balancer:             &lb.RoundRobinLoadBalancer{},
		spec:                 ServiceSpec{ServiceName: "service"},
		queryOpts:            &api.QueryOptions{},
		prioritizedInstances: make([][]*api.ServiceEntry, 1),
		log:                  log.Printf,
		init:                 make(chan struct{}),
		initDone:             sync.Once{},
	}
	go r.populateFromConsul("dc", 0)

	expected := []ServiceAddress{{"localhost", 8080}, {"localhost2", 8081}}

	for i := 0; i < 100; i++ {
		go func() {
			addr, err := r.Resolve(context.Background())
			assert.NoError(t, err)
			assert.Contains(t, expected, addr)
		}()
	}

}

//nolint:funlen
func TestServiceResolver_getTargetsForUpdate(t *testing.T) {
	r := &ServiceResolver{
		prioritizedInstances: make([][]*api.ServiceEntry, 0),
	}

	type args struct {
		se       []*api.ServiceEntry
		priority int
	}
	tests := []struct {
		state            [][]*api.ServiceEntry
		name             string
		args             args
		wantTargets      []*api.ServiceEntry
		wantShouldUpdate bool
	}{
		{
			name: "highest priority nodes changed - should return input",
			state: [][]*api.ServiceEntry{
				{
					{
						Service: &api.AgentService{ID: "1"},
					},
				},
				{
					{
						Service: &api.AgentService{ID: "2"},
					},
				},
			},
			args: args{
				se:       []*api.ServiceEntry{{Service: &api.AgentService{ID: "1"}}, {Service: &api.AgentService{ID: "2"}}},
				priority: 0,
			},
			wantTargets:      []*api.ServiceEntry{{Service: &api.AgentService{ID: "1"}}, {Service: &api.AgentService{ID: "2"}}},
			wantShouldUpdate: true,
		},
		{
			name: "has high priority nodes and lowest priority nodes changed - should return nil",
			state: [][]*api.ServiceEntry{
				{
					{
						Service: &api.AgentService{ID: "1"},
					},
				},
				{
					{
						Service: &api.AgentService{ID: "2"},
					},
				},
			},
			args: args{
				se:       []*api.ServiceEntry{{Service: &api.AgentService{ID: "1"}}, {Service: &api.AgentService{ID: "2"}}},
				priority: 1,
			},
			wantTargets:      nil,
			wantShouldUpdate: false,
		},
		{
			name: "no high priority nodes left - should return low priority nodes",
			state: [][]*api.ServiceEntry{
				{
					{
						Service: &api.AgentService{ID: "1"},
					},
				},
				{
					{
						Service: &api.AgentService{ID: "2"},
					},
				},
			},
			args: args{
				se:       nil,
				priority: 0,
			},
			wantTargets:      []*api.ServiceEntry{{Service: &api.AgentService{ID: "2"}}},
			wantShouldUpdate: true,
		},
		{
			name: "no nodes left - should return nil slice and true",
			state: [][]*api.ServiceEntry{
				{},
				{
					{
						Service: &api.AgentService{ID: "2"},
					},
				},
			},
			args: args{
				se:       nil,
				priority: 1,
			},
			wantTargets:      nil,
			wantShouldUpdate: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			r.prioritizedInstances = tt.state
			targets, shouldUpdate := r.getTargetsForUpdate(tt.args.se, tt.args.priority)
			assert.Equalf(t, tt.wantTargets, targets, "getTargetsForUpdate(%v, %v)", tt.args.se, tt.args.priority)
			assert.Equalf(t, tt.wantShouldUpdate, shouldUpdate, "getTargetsForUpdate(%v, %v)", tt.args.se, tt.args.priority)
		})
	}
}
