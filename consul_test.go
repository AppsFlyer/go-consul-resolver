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

func (c *MockClient) ServiceMultipleTags(service string, tags []string, passingOnly bool, q *api.QueryOptions) (
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
		client:    c,
		ctx:       context.Background(),
		balancer:  &lb.RoundRobinLoadBalancer{},
		spec:      ServiceSpec{ServiceName: "service"},
		queryOpts: &api.QueryOptions{},
		log:       log.Printf,
		init:      make(chan struct{}),
		initDone:  sync.Once{},
	}
	go r.populateFromConsul()

	expected := []ServiceAddress{{"localhost", 8080}, {"localhost2", 8081}}

	for i := 0; i < 100; i++ {
		go func() {
			addr, err := r.Resolve(context.Background())
			assert.NoError(t, err)
			assert.Contains(t, expected, addr)
		}()
	}

}
