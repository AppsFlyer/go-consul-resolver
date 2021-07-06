package test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/testcontainers/testcontainers-go"
)

func createNetwork(name string) (testcontainers.Network, error) {
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return nil, err
	}

	nreq := testcontainers.NetworkRequest{
		Internal:   false,
		Name:       name,
		Attachable: true,
		SkipReaper: true,
	}

	return provider.CreateNetwork(context.Background(), nreq)
}

func startConsulContainer(t *testing.T, network string) testcontainers.Container {
	req := &testcontainers.ContainerRequest{
		Image:        "consul:1.9.5",
		Env:          map[string]string{"CONSUL_BIND_INTERFACE": "eth0"},
		ExposedPorts: []string{"8500"},
		Networks:     []string{network},
		Name:         "consul",
	}

	return startContainer(t, req)
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

func registerServiceInConsul(id int, name string, tags []string, c testcontainers.Container, client *api.Client) error {
	port, err := c.MappedPort(context.Background(), "8080")
	if err != nil {
		return err
	}

	return client.Agent().ServiceRegister(&api.AgentServiceRegistration{
		ID:      fmt.Sprintf("%d", id),
		Name:    name,
		Port:    port.Int(),
		Address: getDockerHost(),
		Tags:    tags,
	})
}

func getDockerHost() string {
	host := os.Getenv("DOCKER_HOST")
	if host != "" {
		dockerURL, _ := url.Parse(host)
		return dockerURL.Hostname()
	}
	return "localhost"
}
