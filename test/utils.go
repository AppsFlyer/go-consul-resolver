package test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/testcontainers/testcontainers-go"
)

func startConsulContainers(t *testing.T, network string, dcs []string) []testcontainers.Container {

	containers := make([]testcontainers.Container, 0, len(dcs))
	for i, dc := range dcs {
		req := &testcontainers.ContainerRequest{
			Image:        "consul:1.9.5",
			Env:          map[string]string{"CONSUL_BIND_INTERFACE": "eth0"},
			ExposedPorts: []string{"8500"},
			Networks:     []string{network},
			Name:         fmt.Sprintf("consul_%d", i),
			Cmd:          []string{"agent", "-dev", fmt.Sprintf("-datacenter=%s", dc), "-client", "0.0.0.0"},
		}
		containers = append(containers, startContainer(t, req))
	}

	// join consul datacenters
	_, err := containers[0].Exec(context.Background(), []string{"consul", "join", "-wan", "consul_1"})
	if err != nil {
		t.Fatal(err)
	}

	return containers
}

func startServiceContainers(t *testing.T, num int, network string) (res []testcontainers.Container) {
	for i := 0; i < num; i++ {
		req := &testcontainers.ContainerRequest{
			FromDockerfile: testcontainers.FromDockerfile{
				Context: "./docker",
			},
			Env:          map[string]string{"INSTANCE_ID": fmt.Sprintf("%d", i)},
			ExposedPorts: []string{"8080"},
			Networks:     []string{network},
			Name:         fmt.Sprintf("service_%d", i),
		}
		res = append(res, startContainer(t, req))
	}
	return
}

func startContainer(t *testing.T, req *testcontainers.ContainerRequest) testcontainers.Container {
	c, err := testcontainers.GenericContainer(context.Background(), testcontainers.GenericContainerRequest{
		ContainerRequest: *req,
		Started:          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func deregisterServiceInConsul(name string, client *api.Client) error {
	return client.Agent().ServiceDeregister(name)
}

func registerServiceInConsul(id int, name string, tags []string, client *api.Client) error { //nolint:unparam too sensitive
	return client.Agent().ServiceRegister(&api.AgentServiceRegistration{
		ID:      fmt.Sprintf("%d", id),
		Name:    name,
		Port:    8080,
		Address: fmt.Sprintf("service_%d", id),
		Tags:    tags,
	})
}
