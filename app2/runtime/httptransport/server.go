// Package httptransport exposes the Lyra protocol over streamable HTTP.
package httptransport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/dispatch"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/rpcwire"
	"go.opentelemetry.io/otel/propagation"
)

const (
	PathRPC       = "/v2/rpc"
	PathInfo      = "/v2/info"
	PathLiveness  = "/v2/health/live"
	PathReadiness = "/v2/health/ready"
)

type Dispatcher interface {
	Dispatch(context.Context, rpcwire.Message, dispatch.Metadata) dispatch.Result
}

type EndpointAuthentication string

const (
	AuthenticationNone       EndpointAuthentication = "none"
	AuthenticationLocalToken EndpointAuthentication = "localToken"
)

type EndpointSpec struct {
	Name             string
	Method           string
	Path             string
	Authentication   EndpointAuthentication
	ResponseStatuses []int
	ResponseType     reflect.Type
}

type endpointRegistration struct {
	spec   EndpointSpec
	handle func(*Server, http.ResponseWriter, *http.Request)
}

var endpointRegistry = []endpointRegistration{
	{
		spec:   EndpointSpec{Name: "rpc", Method: http.MethodPost, Path: PathRPC, Authentication: AuthenticationLocalToken, ResponseStatuses: []int{http.StatusOK, http.StatusNoContent}},
		handle: (*Server).serveRPC,
	},
	{
		spec:   EndpointSpec{Name: "info", Method: http.MethodGet, Path: PathInfo, Authentication: AuthenticationNone, ResponseStatuses: []int{http.StatusOK}, ResponseType: reflect.TypeFor[RuntimeInfo]()},
		handle: (*Server).serveInfo,
	},
	{
		spec:   EndpointSpec{Name: "liveness", Method: http.MethodGet, Path: PathLiveness, Authentication: AuthenticationNone, ResponseStatuses: []int{http.StatusOK}, ResponseType: reflect.TypeFor[LivenessStatus]()},
		handle: (*Server).serveLiveness,
	},
	{
		spec:   EndpointSpec{Name: "readiness", Method: http.MethodGet, Path: PathReadiness, Authentication: AuthenticationNone, ResponseStatuses: []int{http.StatusOK, http.StatusServiceUnavailable}, ResponseType: reflect.TypeFor[ReadinessStatus]()},
		handle: (*Server).serveReadiness,
	},
}

func Contract() []EndpointSpec {
	contract := make([]EndpointSpec, len(endpointRegistry))
	for index, endpoint := range endpointRegistry {
		contract[index] = endpoint.spec
		contract[index].ResponseStatuses = slices.Clone(endpoint.spec.ResponseStatuses)
	}
	return contract
}

type Config struct {
	Dispatcher   Dispatcher
	ServerInfo   protocol.ServerInfo
	LocalToken   string
	CORSOrigins  []string
	HealthProbes []HealthProbe
	Logger       *slog.Logger
}

type Server struct {
	dispatcher Dispatcher
	info       RuntimeInfo
	serverID   string
	token      string
	origins    []string
	probes     []*probeRunner
	logger     *slog.Logger
	propagator propagation.TextMapPropagator

	httpServer *http.Server
	shutdown   chan struct{}
	stopOnce   sync.Once
	serveMu    sync.Mutex
	served     bool
	requestIDs atomic.Uint64
}

func New(config Config) (*Server, error) {
	if config.Dispatcher == nil {
		return nil, errors.New("httptransport: dispatcher is required")
	}
	if config.ServerInfo.InstanceID == "" || config.ServerInfo.Name == "" || config.ServerInfo.Version == "" {
		return nil, errors.New("httptransport: complete server identity is required")
	}
	if err := validateOrigins(config.CORSOrigins); err != nil {
		return nil, err
	}
	probes, err := newProbeRunners(config.HealthProbes)
	if err != nil {
		return nil, err
	}
	server := &Server{
		dispatcher: config.Dispatcher,
		info:       newRuntimeInfo(config.ServerInfo),
		serverID:   config.ServerInfo.Name + "/" + config.ServerInfo.Version,
		token:      config.LocalToken,
		origins:    slices.Clone(config.CORSOrigins),
		probes:     probes,
		logger:     config.Logger,
		propagator: propagation.TraceContext{},
		shutdown:   make(chan struct{}),
	}
	if server.logger == nil {
		server.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	server.httpServer = &http.Server{
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}
	return server, nil
}

func DefaultCORSOrigins() []string {
	return []string{"wails://localhost", "http://wails.localhost", "http://127.0.0.1:5174", "http://localhost:5173"}
}

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, endpoint := range endpointRegistry {
		handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			endpoint.handle(server, response, request)
		})
		if endpoint.spec.Authentication == AuthenticationLocalToken {
			handler = server.requireToken(handler)
		}
		mux.Handle(endpoint.spec.Method+" "+endpoint.spec.Path, handler)
	}
	return server.withLifecycle(server.withTelemetry(server.withIdentity(server.withCORS(mux))))
}

func (server *Server) Serve(listener net.Listener) error {
	if listener == nil {
		return errors.New("httptransport: listener is required")
	}
	server.serveMu.Lock()
	if server.served {
		server.serveMu.Unlock()
		return errors.New("httptransport: Serve may be called only once")
	}
	server.served = true
	server.serveMu.Unlock()
	err := server.httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (server *Server) Shutdown(ctx context.Context) error {
	server.stopOnce.Do(func() { close(server.shutdown) })
	return server.httpServer.Shutdown(ctx)
}

func (server *Server) Close() error {
	server.stopOnce.Do(func() { close(server.shutdown) })
	return server.httpServer.Close()
}

func (server *Server) withLifecycle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithCancel(request.Context())
		joined := make(chan struct{})
		go func() {
			select {
			case <-server.shutdown:
				cancel()
			case <-ctx.Done():
			}
			close(joined)
		}()
		next.ServeHTTP(response, request.WithContext(ctx))
		cancel()
		<-joined
	})
}

func (server *Server) withIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Request-Id", fmt.Sprintf("req_%016x", server.requestIDs.Add(1)))
		response.Header().Set("X-Server", server.serverID)
		next.ServeHTTP(response, request)
	})
}

func validateOrigins(origins []string) error {
	seen := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		if strings.TrimSpace(origin) == "" || origin != strings.TrimSpace(origin) {
			return errors.New("httptransport: CORS origins must be non-empty and trimmed")
		}
		if _, exists := seen[origin]; exists {
			return fmt.Errorf("httptransport: duplicate CORS origin %q", origin)
		}
		seen[origin] = struct{}{}
	}
	return nil
}
