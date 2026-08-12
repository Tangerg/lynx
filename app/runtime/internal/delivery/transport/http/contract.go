package http

import (
	"net/http"
	"reflect"
	"slices"

	"github.com/go-chi/chi/v5"
)

// EndpointKind separates the JSON-RPC binding from typed operational sidecars.
// It is a transport fact: neither Application nor the protocol method registry
// needs to know which HTTP path carries an operation.
type EndpointKind string

const (
	EndpointKindRPC     EndpointKind = "rpc"
	EndpointKindSidecar EndpointKind = "sidecar"
)

// EndpointAuthentication states the HTTP-layer gate applied before a handler.
type EndpointAuthentication string

const (
	EndpointAuthenticationNone       EndpointAuthentication = "none"
	EndpointAuthenticationLocalToken EndpointAuthentication = "localToken"
)

const (
	endpointRPC       = "rpc"
	endpointInfo      = "info"
	endpointLiveness  = "liveness"
	endpointReadiness = "readiness"
)

// EndpointSpec is one implemented HTTP entrypoint. ResponseType is nil for the
// JSON-RPC binding because its typed method results live in operation.Contract;
// sidecars name their exact flat-JSON response type here.
type EndpointSpec struct {
	Name             string
	Kind             EndpointKind
	Method           string
	Path             string
	Authentication   EndpointAuthentication
	ResponseStatuses []int
	ResponseType     reflect.Type
}

type endpointRegistration struct {
	EndpointSpec
	handler func(*Server, http.ResponseWriter, *http.Request)
}

// EnumSpec supplies the closed string sets used only by sidecar response DTOs.
// The JSON-RPC registry owns its own enums through contractcatalog.
type EnumSpec struct {
	Type   reflect.Type
	Values []string
}

// HTTPContract is the delivery-owned machine description used by both routing
// helpers and the out-of-graph contract generator.
type HTTPContract struct {
	Endpoints []EndpointSpec
	Enums     []EnumSpec
}

var endpointRegistry = struct {
	Endpoints []endpointRegistration
	Enums     []EnumSpec
}{
	Endpoints: []endpointRegistration{
		{
			EndpointSpec: EndpointSpec{
				Name:             endpointRPC,
				Kind:             EndpointKindRPC,
				Method:           http.MethodPost,
				Path:             "/v2/rpc",
				Authentication:   EndpointAuthenticationLocalToken,
				ResponseStatuses: []int{http.StatusOK, http.StatusAccepted, http.StatusNoContent},
			},
			handler: (*Server).serveRPC,
		},
		{
			EndpointSpec: EndpointSpec{
				Name:             endpointInfo,
				Kind:             EndpointKindSidecar,
				Method:           http.MethodGet,
				Path:             "/v2/info",
				Authentication:   EndpointAuthenticationNone,
				ResponseStatuses: []int{http.StatusOK},
				ResponseType:     reflect.TypeFor[RuntimeInfo](),
			},
			handler: (*Server).handleInfo,
		},
		{
			EndpointSpec: EndpointSpec{
				Name:             endpointLiveness,
				Kind:             EndpointKindSidecar,
				Method:           http.MethodGet,
				Path:             "/v2/health/live",
				Authentication:   EndpointAuthenticationNone,
				ResponseStatuses: []int{http.StatusOK},
				ResponseType:     reflect.TypeFor[LivenessStatus](),
			},
			handler: (*Server).handleLiveness,
		},
		{
			EndpointSpec: EndpointSpec{
				Name:             endpointReadiness,
				Kind:             EndpointKindSidecar,
				Method:           http.MethodGet,
				Path:             "/v2/health/ready",
				Authentication:   EndpointAuthenticationNone,
				ResponseStatuses: []int{http.StatusOK, http.StatusServiceUnavailable},
				ResponseType:     reflect.TypeFor[ReadinessStatus](),
			},
			handler: (*Server).handleReadiness,
		},
	},
	Enums: []EnumSpec{
		{Type: reflect.TypeFor[HealthStatus](), Values: []string{string(HealthOK), string(HealthDegraded), string(HealthUnhealthy)}},
		{Type: reflect.TypeFor[LivenessState](), Values: []string{string(LivenessOK)}},
		{Type: reflect.TypeFor[HTTPTransportKind](), Values: []string{string(HTTPTransport)}},
	},
}

// Contract returns an isolated snapshot so generators and tests cannot mutate
// the process's routing authority.
func Contract() HTTPContract {
	contract := HTTPContract{
		Endpoints: make([]EndpointSpec, len(endpointRegistry.Endpoints)),
		Enums:     make([]EnumSpec, len(endpointRegistry.Enums)),
	}
	for index, registration := range endpointRegistry.Endpoints {
		endpoint := registration.EndpointSpec
		endpoint.ResponseStatuses = slices.Clone(endpoint.ResponseStatuses)
		contract.Endpoints[index] = endpoint
	}
	for index, enum := range endpointRegistry.Enums {
		enum.Values = slices.Clone(enum.Values)
		contract.Enums[index] = enum
	}
	return contract
}

func endpointNamed(name string) EndpointSpec {
	for _, registration := range endpointRegistry.Endpoints {
		if registration.Name == name {
			return registration.EndpointSpec
		}
	}
	panic("http: unknown endpoint " + name)
}

func endpointPath(name string) string {
	return endpointNamed(name).Path
}

func isPublicEndpointPath(path string) bool {
	for _, registration := range endpointRegistry.Endpoints {
		if registration.Path == path {
			return registration.Authentication == EndpointAuthenticationNone
		}
	}
	return false
}

func registerEndpoints(router *chi.Mux, server *Server) {
	for _, endpoint := range endpointRegistry.Endpoints {
		handler := endpoint.handler
		router.MethodFunc(endpoint.Method, endpoint.Path, func(w http.ResponseWriter, r *http.Request) {
			handler(server, w, r)
		})
	}
}
