package a2a

import (
	"encoding/json"
	"fmt"
	"net/http"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// DefaultRPCPattern is where [NewHTTPHandler] mounts the JSON-RPC method
// endpoint. The AgentCard's JSON-RPC interface URL must point at this path.
const DefaultRPCPattern = "/invoke"

// ServerConfig wires a [Agent] into an HTTP A2A endpoint.
type ServerConfig struct {
	// Agent is the capability served over A2A. Required.
	Agent Agent

	// Card is the AgentCard served at the well-known path. Required and
	// snapshotted during construction — its SupportedInterfaces should advertise
	// a JSON-RPC interface whose URL ends in RPCPattern. Build it with
	// [JSONRPCInterface] for the transport entry.
	Card *sdka2a.AgentCard

	// RPCPattern overrides where the JSON-RPC endpoint is mounted. Empty
	// uses [DefaultRPCPattern].
	RPCPattern string

	// HandlerOptions are forwarded to [a2asrv.NewHandler] (task store, push
	// notifications, interceptors, ...). Empty uses the SDK defaults
	// (in-memory task store).
	HandlerOptions []a2asrv.RequestHandlerOption
}

// NewHTTPHandler builds an http.Handler serving the A2A protocol for a lynx
// [Agent]: the JSON-RPC method endpoint at RPCPattern and the AgentCard at
// [a2asrv.WellKnownAgentCardPath]. Mount it on a server, or compose it into
// a larger mux.
//
// The transport is JSON-RPC over HTTP.
func NewHTTPHandler(cfg ServerConfig) (http.Handler, error) {
	exec, err := newExecutor(cfg.Agent)
	if err != nil {
		return nil, err
	}
	if cfg.Card == nil {
		return nil, ErrNilCard
	}
	card, err := snapshotAgentCard(cfg.Card)
	if err != nil {
		return nil, err
	}
	if cfg.RPCPattern == "" {
		cfg.RPCPattern = DefaultRPCPattern
	}

	requestHandler := a2asrv.NewHandler(exec, cfg.HandlerOptions...)

	mux := http.NewServeMux()
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(card))
	if err := registerRPCHandler(mux, cfg.RPCPattern, a2asrv.NewJSONRPCHandler(requestHandler)); err != nil {
		return nil, err
	}
	return mux, nil
}

func snapshotAgentCard(card *sdka2a.AgentCard) (*sdka2a.AgentCard, error) {
	data, err := json.Marshal(card)
	if err != nil {
		return nil, fmt.Errorf("%w %q: encode: %w", ErrInvalidCard, card.Name, err)
	}
	var snapshot sdka2a.AgentCard
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("%w %q: decode snapshot: %w", ErrInvalidCard, card.Name, err)
	}
	return &snapshot, nil
}

func registerRPCHandler(mux *http.ServeMux, pattern string, handler http.Handler) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w %q: %v", ErrInvalidRPCPattern, pattern, recovered)
		}
	}()
	mux.Handle(pattern, handler)
	return nil
}

// JSONRPCInterface is a small helper for building an AgentCard: it declares a
// JSON-RPC transport interface at url (which should end in the server's
// RPCPattern). Mirrors the SDK's a2a.NewAgentInterface with the protocol
// fixed to JSON-RPC.
func JSONRPCInterface(url string) *sdka2a.AgentInterface {
	return sdka2a.NewAgentInterface(url, sdka2a.TransportProtocolJSONRPC)
}
