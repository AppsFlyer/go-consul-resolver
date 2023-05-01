package test

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	consulresolver "github.com/AppsFlyer/go-consul-resolver"
	"github.com/AppsFlyer/go-consul-resolver/lb"
	"github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
)

const (
	serviceName = "hello-service"
	consulName  = "consul_1"
	consulPort  = 8500
)

type Suite struct {
	suite.Suite
	serviceContainers []testcontainers.Container
	consulContainers  []testcontainers.Container
	consulClient      *api.Client
}

func (s *Suite) SetupSuite() {

	dockerNetwork := os.Getenv("DOCKER_NETWORK")
	if dockerNetwork == "" {
		s.T().Fatal("could not determine docker network")
	}

	consulContainers := startConsulContainers(s.T(), dockerNetwork, []string{"dc1", "dc2"})
	s.consulContainers = consulContainers

	serviceContainers := startServiceContainers(s.T(), 3, dockerNetwork)
	s.serviceContainers = append(s.serviceContainers, serviceContainers...)

	consulClient, err := api.NewClient(&api.Config{Address: fmt.Sprintf("%s:%d", consulName, consulPort)})
	if err != nil {
		s.T().Fatal(err)
	}
	s.consulClient = consulClient
}

func (s *Suite) TearDownSuite() {
	ctx := context.Background()
	for i := 0; i < len(s.serviceContainers); i++ {
		_ = s.serviceContainers[i].Terminate(ctx)
	}
	for i := 0; i < len(s.consulContainers); i++ {
		_ = s.consulContainers[i].Terminate(ctx)
	}

}

func (s *Suite) TearDownTest() {
	for i := range s.serviceContainers {
		if err := deregisterServiceInConsul(fmt.Sprintf("%d", i), s.consulClient); err != nil {
			s.T().Fatal(err)
		}
	}
}

func (s *Suite) TestRoundRobinLoadBalancedClient() {

	for i := range s.serviceContainers {
		if err := registerServiceInConsul(i, serviceName, nil, s.consulClient); err != nil {
			s.T().Fatal(err)
		}
	}

	s.Assert().Eventually(func() bool {
		svcs, _, err := s.consulClient.Catalog().Service(serviceName, "", nil)
		return len(svcs) == 3 && err == nil
	},
		10*time.Second,
		1*time.Second)

	consulClient, _ := api.NewClient(&api.Config{Address: fmt.Sprintf("%s:%d", consulName, consulPort)})
	coolServiceResolver, _ := consulresolver.NewConsulResolver(context.Background(), consulresolver.ResolverConfig{
		Log: log.Printf,
		ServiceSpec: consulresolver.ServiceSpec{
			ServiceName: serviceName,
		},
		Balancer: &lb.RoundRobinLoadBalancer{},
		Client:   consulClient,
	})

	transport, _ := consulresolver.NewLoadBalancedTransport(
		consulresolver.TransportConfig{
			Resolvers: []consulresolver.Resolver{coolServiceResolver},
			Log:       log.Printf,
		})

	client := &http.Client{Transport: transport}

	results := s.executeServiceRequests(len(s.serviceContainers), client)
	s.Assert().Equal(map[string]int{"0": 1, "1": 1, "2": 1}, results)
}

func (s *Suite) TestTagAwareLoadBalancedClient() {

	// Register each service with a different tag
	for i := range s.serviceContainers {
		if err := registerServiceInConsul(i, serviceName, []string{fmt.Sprintf("%d", i)}, s.consulClient); err != nil {
			s.T().Fatal(err)
		}
	}

	s.Assert().Eventually(func() bool {
		svcs, _, err := s.consulClient.Catalog().Service(serviceName, "", nil)
		return len(svcs) == 3 && err == nil
	},
		10*time.Second,
		1*time.Second)

	consulClient, _ := api.NewClient(&api.Config{Address: fmt.Sprintf("%s:%d", consulName, consulPort)})
	coolServiceResolver, _ := consulresolver.NewConsulResolver(context.Background(), consulresolver.ResolverConfig{
		Log: log.Printf,
		ServiceSpec: consulresolver.ServiceSpec{
			ServiceName: serviceName,
		},
		Balancer: &lb.TagAwareLoadBalancer{Tags: []string{"1"}, FallbackAllowed: true},
		Client:   consulClient,
	})

	transport, _ := consulresolver.NewLoadBalancedTransport(
		consulresolver.TransportConfig{
			Resolvers: []consulresolver.Resolver{coolServiceResolver},
			Log:       log.Printf,
		})

	client := &http.Client{Transport: transport}

	results := s.executeServiceRequests(len(s.serviceContainers), client)
	s.Assert().Equal(map[string]int{"1": 3}, results)

	// Deregister the service with tag "1" and assert that we fallback to round robin
	if err := deregisterServiceInConsul("1", s.consulClient); err != nil {
		s.T().Fatal(err)
	}

	// Wait for resolver to be notified
	time.Sleep(1 * time.Second)

	results = s.executeServiceRequests(4, client)
	s.Assert().Equal(map[string]int{"0": 2, "2": 2}, results)

}

func (s *Suite) executeServiceRequests(num int, client *http.Client) map[string]int {
	results := make(map[string]int)
	for i := 0; i < num; i++ {
		res, err := client.Get(fmt.Sprintf("http://%s/hello", serviceName))
		if err != nil {
			s.T().Fatal(err)
		}
		bodyBytes, err := io.ReadAll(res.Body)
		if err != nil {
			s.T().Fatal(err)
		}
		res.Body.Close()

		instanceID := strings.Split(string(bodyBytes), ", ")[1]
		results[instanceID]++
	}
	return results
}

func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}
