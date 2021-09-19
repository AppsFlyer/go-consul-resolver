# Go Consul Resolver
[![Actions Status](https://github.com/AppsFlyer/go-consul-resolver/workflows/go-consul-resolver/badge.svg?branch=main)](https://github.com/AppsFlyer/go-consul-resolver/actions)
[![Godocs](https://img.shields.io/badge/golang-documentation-blue.svg)](https://pkg.go.dev/github.com/AppsFlyer/go-consul-resolver)

A library of composable layers that is designed to provide client-side load balancing for (but not limited to) HTTP client-server communication, using Consul as the service discovery layer

[Example Usage](#Example)  
(See advanced usage patterns in the `test` package)

## Components

The library provides three main components which - when combined - allow for transparent load balancing to occur when making HTTP requests.  
The provided implementations rely on Consul as the service discovery backend, however additional providers can be supported.

 [Load Balancer](#load-balancer)

 [Resolver](#resolver)
 
 [Transport](#http-transport)
 
 [Known Limitations](#known-limitations)

### Load Balancer

The load balancer is used to choose the physical node address to dispatch the request to, based on the list of available nodes that it is provided with.  
The default implementation provided out of the box uses a Round Robin algorithm and supports entities based on Consul's API.  
This allows for implementing more advanced load balancers, with awareness to service tags, DC location, etc.

#### Tag Aware Load Balancer
Given a list of tags this load balancer will prefer nodes with the provided tags. If no tagged nodes found and fallback allowed it will choose the next
node using round robin algorithm.


#### Custom Load Balancer
may be added by implementing the `Balancer` API:
 
 ```go
import "github.com/hashicorp/consul/api"

type Balancer interface {
    Select() (*api.ServiceEntry, error)
    UpdateTargets(targets []*api.ServiceEntry)
}
 ```

### Resolver

The resolver is responsible for resolving a service name to physical node addresses, using Consul as the service discovery provider.  
It does so by querying Consul's Health API with blocking queries, which allows it to be notified when changes occur in the list of available nodes for a given service.  

When initializing a new Consul Resolver (via `NewConsulResolver`), you must provide a context and a configuration struct.  
The context is used to gracefully terminate the go routine which is used to watch Consul - note that when the context is cancelled, 
the resolver will become stale and will immediately return an error (based on the context's `Err` output) when trying to use it.

The configuration allows specifying the following parameters:
* ServiceSpec - the spec of the service being resolved (service name, port, etc.)
* Balancer - the load balancer to use
* Client - a Consul API client
* Query - the Consul query options, if you wish to override the defaults
* LogFn - A custom logging function

Once initialized with a load balancer, the resolver can be used as a stand-alone component to load balance between the various instances of the service name it was provided with.

### HTTP Transport

The `LoadBalancedTransport` serves as the intermediary layer between the `Resolver` and a vanilla HTTP client.  
It intercepts outbound HTTP requests and - based on the request's host - delegates IP resolution to the matching resolver, or to the base transport layer if no resolver matches.

The transport implements the `http.RoundTriper` interface, using `*http.Transport` as the underlying implementation by default (unless another implementation was provided).  
By default, the library obtains the base transport from `http.DefaultTransport` however you may provide any type implementing `http.RoundTripper` via the configuration.

Upon successful address resolution from Consul, the transport will replace the Host part of the requests' URL struct with the IP address of the target node. Thus, a request to `http://my-cool-service/api` will be cloned, and its URL changed to `http://10.1.2.3/api`.


The configuration allows specifying the following parameters:
* Resolvers - a list of `Resolver` instances that will be used by the transport to resolve hostnames
* Base - a base `http.RoundTripper` instance to override the default transport
* NetResolverFallback - a boolean flag that controls the transport's behavior in case of a resolution error.  
By default (false), a `Resolver` error will propagate up the call stack, and fail the HTTP request.  
If set to true, the transport will attempt to resolve the address by delegating the request to the base transport implementation (which will resolve it via DNS).
* LogFn - A custom logging function
 

### Known Limitations

* TLS - in order to support TLS, you can provide a custom Base `http.Transport` with the `ServerName` in it's `TLSClientConfig` set to the hostname presented by your certificate.

# Example

```go
package main

import (
	"context"
	"net/http"

	"github.com/hashicorp/consul/api"

	consulresolver "github.com/AppsFlyer/go-consul-resolver"
	"github.com/AppsFlyer/go-consul-resolver/lb"
)

func main() {

	consulClient, _ := api.NewClient(&api.Config{Address: "localhost:8500")})
	coolServiceResolver, _ := consulresolver.NewConsulResolver(context.Background(), consulresolver.ResolverConfig{
		ServiceSpec: consulresolver.ServiceSpec{
			ServiceName: "cool-service",
		},
		Balancer: &lb.TagAwareLoadBalancer{
			Tags: []string{"az-eu-west-1c", "az-eu-east-1c"},
		},
		Client: consulClient,
	})

	transport, _ := consulresolver.NewLoadBalancedTransport(
		consulresolver.TransportConfig{
			Resolvers: []consulresolver.Resolver{coolServiceResolver},
		})

	client := &http.Client{Transport: transport}

	// this will resolve via Consul
	res, _ := client.Get("http://cool-service/_/health.json")

	// this will resolve via DNS 
	res, _ := client.Get("http://google.com")
}
```
