package lb

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hashicorp/consul/api"
)

func TestTagHitOnce(t *testing.T) {
	lb := TagAwareLoadBalancer{
		Tags: []string{"tag1"},
	}
	lb.UpdateTargets(getTargets())

	for i := 0; i < 100; i++ {
		res, err := lb.Select()
		assert.NoError(t, err)
		assert.Equal(t, "1", res.Service.ID)
	}
}

func TestTagNoHitNoFallback(t *testing.T) {
	lb := TagAwareLoadBalancer{
		Tags: []string{"no_tag"},
	}
	lb.UpdateTargets(getTargets())

	for i := 0; i < 100; i++ {
		_, err := lb.Select()
		assert.Error(t, err)
	}
}

func TestTagNoHitWithFallback(t *testing.T) {
	lb := TagAwareLoadBalancer{
		Tags:            []string{"no_tag"},
		FallbackAllowed: true,
	}
	lb.UpdateTargets(getTargets())

	for i := 0; i < 100; i++ {
		_, err := lb.Select()
		assert.NoError(t, err)
	}
}

func TestMultipleTagHits(t *testing.T) {
	const duplicateTag = "duplicate_tag"
	tags := []string{duplicateTag}

	validResults := map[string]struct{}{
		"0": {},
		"5": {},
	}

	targets := getTargets()
	targets[0].Service.Tags = append(targets[0].Service.Tags, duplicateTag)
	targets[5].Service.Tags = append(targets[5].Service.Tags, duplicateTag)
	lb := TagAwareLoadBalancer{
		Tags: tags,
	}
	lb.UpdateTargets(targets)

	for i := 0; i < 1000; i++ {
		res, err := lb.Select()
		assert.NoError(t, err)
		_, ok := validResults[res.Service.ID]
		assert.True(t, ok)
	}
}

func getTargets() []*api.ServiceEntry {
	const count = 10
	res := make([]*api.ServiceEntry, 0, count)

	for i := 0; i < count; i++ {
		index := strconv.Itoa(i)
		res = append(res, &api.ServiceEntry{
			Service: &api.AgentService{
				ID:   index,
				Tags: []string{"tag" + index},
			}})
	}
	return res
}
