package resolver

import (
	"sync"
	"sync/atomic"

	"github.com/friendsofgo/errors"
	"github.com/hashicorp/consul/api"
)

type RoundRobinLoadBalancer struct {
	targets []*api.ServiceEntry
	index   uint64
	mu      sync.RWMutex
}

func (r *RoundRobinLoadBalancer) Select() (*api.ServiceEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.targets) == 0 {
		return nil, errors.New("unable to select target from empty list")
	}

	// select next index % size of targets array
	return r.targets[int(atomic.AddUint64(&r.index, uint64(1))%uint64(len(r.targets)))], nil
}

func (r *RoundRobinLoadBalancer) UpdateTargets(targets []*api.ServiceEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.targets = targets
}
