package consulresolver

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/friendsofgo/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const (
	serviceName = "test-service"
)

type MockResolver struct {
	mock.Mock
}

func (m *MockResolver) Resolve(context.Context) (ServiceAddress, error) {
	args := m.Called()
	addr, err := args.Get(0), args.Get(1)
	if err != nil {
		return ServiceAddress{}, err.(error)
	}

	return addr.(ServiceAddress), nil

}

func (m *MockResolver) ServiceName() string {
	args := m.Called()
	return args.String(0)
}

type TestSuite struct {
	suite.Suite
	resolver *MockResolver
}

func (t *TestSuite) SetupTest() {
	t.resolver = &MockResolver{}
}

func TestTestSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}

func (t *TestSuite) TestResolverSuccess() {
	expectedAddr := ServiceAddress{Host: "service-address", Port: 8080}
	t.resolver.On("Resolve").Return(expectedAddr, nil)
	t.resolver.On("ServiceName").Return(serviceName)

	tr, err := NewLoadBalancedTransport(TransportConfig{
		Resolvers: []Resolver{t.resolver},
	})
	t.Assert().NoError(err)
	client := &http.Client{Transport: tr}
	res, err := client.Get("http://test-service/do/something") //nolint:errcheck
	t.Assert().NoError(err)
	_ = res.Body.Close()
	t.resolver.AssertExpectations(t.T())
}

func (t *TestSuite) TestResolverUnknownService() {
	t.resolver.On("ServiceName").Return(serviceName)
	t.resolver.AssertNotCalled(t.T(), "Resolve")

	tr, err := NewLoadBalancedTransport(TransportConfig{
		Resolvers: []Resolver{t.resolver},
		Base:      getAssertableTransport(t, true),
	})
	t.Assert().NoError(err)
	client := &http.Client{Transport: tr}
	res, err := client.Get("http://other-service/do/something") //nolint:errcheck
	t.Assert().NoError(err)
	_ = res.Body.Close()

	t.resolver.AssertExpectations(t.T())
}

func (t *TestSuite) TestResolverError() {
	t.resolver.On("ServiceName").Return(serviceName)
	t.resolver.On("Resolve").Return(ServiceAddress{}, errors.New("failed"))

	tr, err := NewLoadBalancedTransport(TransportConfig{
		Resolvers: []Resolver{t.resolver},
		Base:      getAssertableTransport(t, false),
	})
	t.Assert().NoError(err)
	client := &http.Client{Transport: tr}
	res, err := client.Get("http://test-service/do/something") //nolint:errcheck
	t.Assert().NoError(err)
	_ = res.Body.Close()

	t.resolver.AssertExpectations(t.T())
}

func (t *TestSuite) TestResolverFallbackOnError() {
	t.resolver.On("ServiceName").Return(serviceName)
	t.resolver.On("Resolve").Return(ServiceAddress{}, errors.New("failed"))

	tr, err := NewLoadBalancedTransport(TransportConfig{
		Resolvers:           []Resolver{t.resolver},
		NetResolverFallback: true,
		Base:                getAssertableTransport(t, true),
	})
	t.Assert().NoError(err)
	client := &http.Client{Transport: tr}
	res, err := client.Get("http://test-service/do/something") //nolint:errcheck
	t.Assert().NoError(err)
	_ = res.Body.Close()

	t.resolver.AssertExpectations(t.T())
}

func getAssertableTransport(t *TestSuite, shouldInvoke bool) *http.Transport {
	base := http.DefaultTransport.(*http.Transport).Clone()
	baseDialCtx := base.DialContext
	base.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if !shouldInvoke {
			t.Fail("dial context invoked, but shouldn't be")
		}
		return baseDialCtx(ctx, network, addr)
	}

	return base
}
