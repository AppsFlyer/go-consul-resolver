package lb

import (
	"math/rand"
	"sync"
	"sync/atomic"

	"github.com/friendsofgo/errors"
	"github.com/hashicorp/consul/api"
)

type TagAwareLoadBalancer struct {
	targets         []*api.ServiceEntry
	mu              sync.RWMutex
	tagsMapping     map[string][]*api.ServiceEntry
	index           uint64
	Tags            []string
	FallbackAllowed bool
}

func (t *TagAwareLoadBalancer) Select() (*api.ServiceEntry, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if len(t.targets) == 0 {
		return nil, errors.New("unable to select target from empty list")
	}

	for _, tag := range t.Tags {
		if targets, ok := t.tagsMapping[tag]; ok && len(targets) > 0 {
			return targets[rand.Intn(len(targets))], nil // nolint:gosec
		}
	}

	if t.FallbackAllowed {
		// select next index % size of targets array
		return t.targets[int(atomic.AddUint64(&t.index, uint64(1))%uint64(len(t.targets)))], nil
	}
	return nil, errors.New("no targets found matching provided tags")

}

func (t *TagAwareLoadBalancer) UpdateTargets(targets []*api.ServiceEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.targets = targets

	newMapping := map[string][]*api.ServiceEntry{}
	for _, tag := range t.Tags {
		newMapping[tag] = []*api.ServiceEntry{}
	}

	for _, target := range targets {
		for _, tag := range target.Service.Tags {
			if _, ok := newMapping[tag]; ok {
				newMapping[tag] = append(newMapping[tag], target)
			}
		}
	}
	t.tagsMapping = newMapping
}
