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
	// [NewJSONRPCInterface] for the transport entry.
	Card *sdka2a.AgentCard

	// RPCPattern overrides where the JSON-RPC endpoint is mounted. Empty
	// uses [DefaultRPCPattern].
	RPCPattern string
}

// NewHTTPHandler returns a plain [http.Handler] rather than starting a server,
// so the host keeps ownership of the listener, TLS, timeouts, and middleware.
// The card is encoded during construction because an AgentCard that cannot be
// marshaled would otherwise fail at the well-known path, where a peer reads it
// as an unreachable agent rather than a misconfigured one.
func NewHTTPHandler(config ServerConfig) (http.Handler, error) {
	exec, err := newExecutor(config.Agent)
	if err != nil {
		return nil, err
	}
	if config.Card == nil {
		return nil, ErrNilCard
	}
	cardHandler, err := newStaticAgentCardHandler(config.Card)
	if err != nil {
		return nil, err
	}
	if config.RPCPattern == "" {
		config.RPCPattern = DefaultRPCPattern
	}

	requestHandler := a2asrv.NewHandler(exec)

	mux := http.NewServeMux()
	mux.Handle(a2asrv.WellKnownAgentCardPath, cardHandler)
	if err := registerRPCHandler(mux, config.RPCPattern, a2asrv.NewJSONRPCHandler(requestHandler)); err != nil {
		return nil, err
	}
	return mux, nil
}

func newStaticAgentCardHandler(card *sdka2a.AgentCard) (http.Handler, error) {
	if _, err := json.Marshal(card); err != nil {
		return nil, fmt.Errorf("%w %q: encode: %w", ErrInvalidCard, card.Name, err)
	}
	return a2asrv.NewStaticAgentCardHandler(card), nil
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

// NewJSONRPCInterface builds the interface entry a card must advertise for the
// endpoint this package mounts. It exists so the transport a peer discovers and
// the transport actually served cannot drift apart in hand-written card
// literals.
func NewJSONRPCInterface(url string) *sdka2a.AgentInterface {
	return sdka2a.NewAgentInterface(url, sdka2a.TransportProtocolJSONRPC)
}
