package a2a

import (
	"net/http"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// DefaultRPCPattern is where [NewHTTPHandler] mounts the JSON-RPC method
// endpoint. The AgentCard's JSON-RPC interface URL must point at this path.
const DefaultRPCPattern = "/invoke"

// ServerConfig wires a lynx [Agent] into an HTTP A2A endpoint.
type ServerConfig struct {
	// Agent is the lynx capability served over A2A. Required.
	Agent Agent

	// Card is the AgentCard served at the well-known path. Required — its
	// SupportedInterfaces should advertise a JSON-RPC interface whose URL
	// ends in RPCPattern. Build it with [JSONRPCInterface] for the
	// transport entry.
	Card *sdka2a.AgentCard

	// RPCPattern overrides where the JSON-RPC endpoint is mounted. Empty
	// uses [DefaultRPCPattern].
	RPCPattern string

	// HandlerOptions are forwarded to [a2asrv.NewHandler] (task store, push
	// notifications, interceptors, ...). Empty uses the SDK defaults
	// (in-memory task store).
	HandlerOptions []a2asrv.RequestHandlerOption
}

// Validate reports whether the config has the required fields.
func (c *ServerConfig) Validate() error {
	if c.Agent == nil {
		return ErrNilAgent
	}
	if c.Card == nil {
		return ErrNilCard
	}
	return nil
}

// ApplyDefaults fills zero fields: RPCPattern defaults to [DefaultRPCPattern].
func (c *ServerConfig) ApplyDefaults() {
	if c.RPCPattern == "" {
		c.RPCPattern = DefaultRPCPattern
	}
}

// NewHTTPHandler builds an http.Handler serving the A2A protocol for a lynx
// [Agent]: the JSON-RPC method endpoint at RPCPattern and the AgentCard at
// [a2asrv.WellKnownAgentCardPath]. Mount it on a server, or compose it into
// a larger mux.
//
// The transport is JSON-RPC over HTTP. Callers needing REST or gRPC can use
// [NewExecutor] with the SDK's other transport handlers directly.
func NewHTTPHandler(cfg ServerConfig) (http.Handler, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	exec, err := NewExecutor(cfg.Agent)
	if err != nil {
		return nil, err
	}
	requestHandler := a2asrv.NewHandler(exec, cfg.HandlerOptions...)

	mux := http.NewServeMux()
	mux.Handle(cfg.RPCPattern, a2asrv.NewJSONRPCHandler(requestHandler))
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(cfg.Card))
	return mux, nil
}

// JSONRPCInterface is a small helper for building an AgentCard: it declares a
// JSON-RPC transport interface at url (which should end in the server's
// RPCPattern). Mirrors the SDK's a2a.NewAgentInterface with the protocol
// fixed to JSON-RPC.
func JSONRPCInterface(url string) *sdka2a.AgentInterface {
	return sdka2a.NewAgentInterface(url, sdka2a.TransportProtocolJSONRPC)
}
