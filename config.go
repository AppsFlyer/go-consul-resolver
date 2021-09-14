package consulresolver

import (
	"net/http"

	"github.com/hashicorp/consul/api"
)

type LogFn func(format string, args ...interface{})

type ServiceSpec struct {
	// The name of the service in Consul.
	// Mandatory
	ServiceName string
	// The port to use, if different from the `Service.Port` in Consul
	// If set to a value other than 0, this will override the service port discovered in Consul.
	// Optional
	// Default: 0
	ServicePort int
	// Filter service instances by Consul tags.
	// Optional
	// Default: nil
	Tags []string
	// Filter service instances by Health status.
	// Optional
	// Default: false (only healthy endpoints are used)
	IncludeUnhealthy bool
}

type TransportConfig struct {
	// A function that will be used for logging.
	// Optional
	// Default: log.Printf
	Log LogFn
	// The resolvers to be used for address resolution.
	// Multiple resolvers are supported, and will be looked up by the `ServiceName`
	// Mandatory
	Resolvers []Resolver
	// If true, the transport will fallback to net/Resolver on resolver error
	// Optional
	// Default: false
	NetResolverFallback bool
	// A base transport to be used for the underlying request handling.
	// Optional
	// Default: http.DefaultTransport
	Base http.RoundTripper
}

type ResolverConfig struct {
	// A function that will be used for logging.
	// Optional
	// Default: log.Printf
	Log LogFn
	// The Service Spec the resolver will handle
	// Mandatory
	ServiceSpec ServiceSpec
	// The Balancer that will be used to select targets
	// Optional
	// Default: RoundRobinLoadBalancer
	Balancer Balancer
	// The consul client
	// Mandatory
	Client *api.Client
	// The consul query options configuration
	// Optional
	Query *api.QueryOptions
}
