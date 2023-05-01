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

	"github.com/google/uuid"

	consulresolver "github.com/AppsFlyer/go-consul-resolver"
	"github.com/AppsFlyer/go-consul-resolver/lb"
	"github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
)

const (
	serviceName = "hello-service"
)

type Suite struct {
	suite.Suite
	serviceContainers []testcontainers.Container
	consulContainers  []testcontainers.Container
	consulClients     []*api.Client
	network           testcontainers.Network
}

func (s *Suite) SetupSuite() {
	var isLocal bool
	dockerNetwork := os.Getenv("DOCKER_NETWORK")
	isLocal = dockerNetwork == ""
	if isLocal {
		runID, _ := uuid.NewRandom()
		dockerNetwork = runID.String()
		network, err := createNetwork(dockerNetwork)
		if err != nil {
			s.T().Fatal("could not create docker network")
		}
		s.network = network
	}

	var dcs = []string{"dc1", "dc2"}

	consulContainers := startConsulContainers(s.T(), dockerNetwork, dcs)
	s.consulContainers = consulContainers

	serviceContainers := startServiceContainers(s.T(), 3, dockerNetwork)
	s.serviceContainers = append(s.serviceContainers, serviceContainers...)

	for i := range dcs {
		host := fmt.Sprintf("consul_%d", i)
		port := "8500"

		if isLocal {
			host = "localhost"
			port = getContainerMappedPort(s.T(), s.consulContainers[i], "8500")
		}

		client, err := api.NewClient(&api.Config{Address: fmt.Sprintf("%s:%s", host, port)})
		if err != nil {
			s.T().Fatal(err)
		}
		s.consulClients = append(s.consulClients, client)
	}
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
		for _, c := range s.consulClients {
			if err := deregisterServiceInConsul(fmt.Sprintf("%d", i), c); err != nil {
				s.T().Fatal(err)
			}
		}
	}
}

func (s *Suite) TestDatacenterAwareLoadBalancedClient() {

	// Register instances 0 and 1 in DC1, register service 2 in DC2

	for i := 0; i < 2; i++ {
		if err := registerServiceInConsul(i, serviceName, nil, s.consulClients[0]); err != nil {
			s.T().Fatal(err)
		}
	}
	if err := registerServiceInConsul(2, serviceName, nil, s.consulClients[1]); err != nil {
		s.T().Fatal(err)
	}

	s.Assert().Eventually(func() bool {
		svcs, _, err := s.consulClients[0].Catalog().Service(serviceName, "", nil)
		return len(svcs) == 2 && err == nil
	},
		10*time.Second,
		1*time.Second)

	s.Assert().Eventually(func() bool {
		svcs, _, err := s.consulClients[1].Catalog().Service(serviceName, "", nil)
		return len(svcs) == 1 && err == nil
	},
		10*time.Second,
		1*time.Second)

	coolServiceResolver, _ := consulresolver.NewConsulResolver(context.Background(), consulresolver.ResolverConfig{
		Log: log.Printf,
		ServiceSpec: consulresolver.ServiceSpec{
			ServiceName: serviceName,
		},
		Balancer:            &lb.RoundRobinLoadBalancer{},
		Client:              s.consulClients[0],
		FallbackDatacenters: []string{"dc2"},
	})

	transport, _ := consulresolver.NewLoadBalancedTransport(
		consulresolver.TransportConfig{
			Resolvers: []consulresolver.Resolver{coolServiceResolver},
			Log:       log.Printf,
		})

	client := &http.Client{Transport: transport}

	results := s.executeServiceRequests(4, client)
	s.Assert().Equal(map[string]int{"0": 2, "1": 2, "2": 0}, results)
}

func (s *Suite) TestRoundRobinLoadBalancedClient() {

	for i := range s.serviceContainers {
		if err := registerServiceInConsul(i, serviceName, nil, s.consulClients[0]); err != nil {
			s.T().Fatal(err)
		}
	}

	s.Assert().Eventually(func() bool {
		svcs, _, err := s.consulClients[0].Catalog().Service(serviceName, "", nil)
		return len(svcs) == 2 && err == nil
	},
		10*time.Second,
		1*time.Second)

	coolServiceResolver, _ := consulresolver.NewConsulResolver(context.Background(), consulresolver.ResolverConfig{
		Log: log.Printf,
		ServiceSpec: consulresolver.ServiceSpec{
			ServiceName: serviceName,
		},
		Balancer: &lb.RoundRobinLoadBalancer{},
		Client:   s.consulClients[0],
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
		if err := registerServiceInConsul(i, serviceName, []string{fmt.Sprintf("%d", i)}, s.consulClients[0]); err != nil {
			s.T().Fatal(err)
		}
	}

	s.Assert().Eventually(func() bool {
		svcs, _, err := s.consulClients[0].Catalog().Service(serviceName, "", nil)
		return len(svcs) == 3 && err == nil
	},
		10*time.Second,
		1*time.Second)

	coolServiceResolver, _ := consulresolver.NewConsulResolver(context.Background(), consulresolver.ResolverConfig{
		Log: log.Printf,
		ServiceSpec: consulresolver.ServiceSpec{
			ServiceName: serviceName,
		},
		Balancer: &lb.TagAwareLoadBalancer{Tags: []string{"1"}, FallbackAllowed: true},
		Client:   s.consulClients[0],
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
	if err := deregisterServiceInConsul("1", s.consulClients[0]); err != nil {
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
