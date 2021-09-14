package consulresolver

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/friendsofgo/errors"
)

type ServiceAddress struct {
	Host string
	Port int
}

type Resolver interface {
	// Resolve should return a ServiceAddress, or a non-nil error if resolution failed
	Resolve(context.Context) (ServiceAddress, error)
	// ServiceName should return the name of the service the resolver is providing targets for
	// Target resolution will be skipped for hosts that do not match the name returned by ServiceName
	ServiceName() string
}

type DialFn func(ctx context.Context, network, addr string) (net.Conn, error)

type LoadBalancedTransport struct {
	resolvers        map[string]Resolver
	base             http.RoundTripper
	log              LogFn
	resolverFallback bool
}

func NewLoadBalancedTransport(conf TransportConfig) (*LoadBalancedTransport, error) {

	if len(conf.Resolvers) == 0 {
		return nil, errors.New("no resolver provided")
	}

	if conf.Log == nil {
		conf.Log = log.Printf
	}

	var base http.RoundTripper
	if conf.Base == nil {
		base = getDefaultTransport()
	} else {
		base = conf.Base
	}

	resolvers := make(map[string]Resolver, len(conf.Resolvers))
	for _, r := range conf.Resolvers {
		resolvers[r.ServiceName()] = r
	}

	return &LoadBalancedTransport{
		resolvers:        resolvers,
		base:             base,
		log:              conf.Log,
		resolverFallback: conf.NetResolverFallback,
	}, nil
}

func (t *LoadBalancedTransport) RoundTrip(req *http.Request) (*http.Response, error) {

	host := strings.Split(req.Host, ":")[0]
	r, ok := t.resolvers[host]
	if !ok {
		t.log("[LoadBalancedTransport] no resolver found for host %s", req.Host)
		return t.base.RoundTrip(req)
	}

	tgt, err := r.Resolve(req.Context())
	if err != nil {
		t.log("[LoadBalancedTransport] failed resolving target - %s", err.Error())
		if t.resolverFallback {
			t.log("[LoadBalancedTransport] falling back to default resolver")
			return t.base.RoundTrip(req)
		}
		return nil, err
	}

	// RoundTrip must not modify the original request - so we clone it
	cloned := req.Clone(req.Context())
	cloned.URL.Host = fmt.Sprintf("%s:%d", tgt.Host, tgt.Port)

	return t.base.RoundTrip(cloned)
}

func getDefaultTransport() *http.Transport {
	if h, ok := http.DefaultTransport.(*http.Transport); ok {
		return h.Clone()
	}
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}
