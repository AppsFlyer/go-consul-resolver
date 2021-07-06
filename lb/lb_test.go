package resolver

import (
	"sync"
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/assert"
)

func TestRoundRobinLoadBalancer(t *testing.T) {
	lb := &RoundRobinLoadBalancer{
		targets: []*api.ServiceEntry{
			{Service: &api.AgentService{ID: "1"}},
			{Service: &api.AgentService{ID: "2"}},
			{Service: &api.AgentService{ID: "3"}},
		},
	}

	for _, id := range []string{"2", "3", "1", "2"} {
		res, err := lb.Select()
		assert.NoError(t, err)
		assert.Equal(t, id, res.Service.ID)
	}
}

func TestRoundRobinLoadBalancerConcurrent(t *testing.T) {
	hits := map[string]int{"1": 0, "2": 0, "3": 0, "4": 0}
	mu := sync.Mutex{}

	lb := &RoundRobinLoadBalancer{
		targets: []*api.ServiceEntry{
			{Service: &api.AgentService{ID: "1"}},
			{Service: &api.AgentService{ID: "2"}},
			{Service: &api.AgentService{ID: "3"}},
			{Service: &api.AgentService{ID: "4"}},
		},
	}

	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := lb.Select()
			assert.NoError(t, err)
			mu.Lock()
			hits[res.Service.ID]++
			mu.Unlock()
		}()
	}

	wg.Wait()
	for _, hits := range hits {
		assert.Equal(t, 25, hits)
	}
}
