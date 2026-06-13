package main

import (
	"net/http"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/Tangerg/lynx/a2a"
	"github.com/Tangerg/lynx/lyra/internal/delivery/protocol"
)

// a2aRPCPattern is the path the A2A JSON-RPC endpoint is mounted at, surfaced
// in the serve banner. Aliases the a2a default so the two stay in lockstep.
const a2aRPCPattern = a2a.DefaultRPCPattern

// buildA2AServer assembles the opt-in A2A endpoint: it exposes this runtime's
// agent ([runtime.Runtime.A2AAgent]) over the A2A protocol on its OWN
// listener (addr), separate from the Lyra Runtime Protocol transport — the
// two are distinct protocols and must not share a mux.
//
// The returned *http.Server is started/stopped by [App.runServer] alongside
// the main transport. addr must be non-empty (the caller gates on it).
func (a *App) buildA2AServer(addr string, info protocol.ServerInfo) (*http.Server, error) {
	card := buildA2ACard(addr, info)
	handler, err := a2a.NewHTTPHandler(a2a.ServerConfig{
		Agent: a.runtime().A2AAgent(),
		Card:  card,
	})
	if err != nil {
		return nil, err
	}
	return &http.Server{Addr: addr, Handler: handler}, nil
}

// buildA2ACard describes this Lyra agent for A2A discovery. The JSON-RPC
// interface URL points at addr + the handler's RPC pattern, so a remote
// client that resolves the well-known card dials the right endpoint.
func buildA2ACard(addr string, info protocol.ServerInfo) *sdka2a.AgentCard {
	name := info.Name
	if name == "" {
		name = "Lyra"
	}
	return &sdka2a.AgentCard{
		Name:                name,
		Description:         "Lyra coding agent runtime — delegate a coding/chat task and receive the reply.",
		Version:             info.Version,
		SupportedInterfaces: []*sdka2a.AgentInterface{a2a.JSONRPCInterface("http://" + addr + a2a.DefaultRPCPattern)},
		DefaultInputModes:   []string{"text"},
		DefaultOutputModes:  []string{"text"},
		Capabilities:        sdka2a.AgentCapabilities{Streaming: true},
		Skills: []sdka2a.AgentSkill{
			{
				ID:          "chat",
				Name:        "Chat",
				Description: "Run a coding/chat task and return the assistant's reply.",
				Tags:        []string{"coding", "chat"},
			},
		},
	}
}
