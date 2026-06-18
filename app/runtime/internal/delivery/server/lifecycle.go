package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// Initialize handles runtime.initialize. Version negotiation
// follows the MCP rules (API.md §2.2): when the client's requested
// version differs from ours, we return the version WE support and
// let the client decide whether to fall back / disconnect.
func (s *Server) Initialize(_ context.Context, in protocol.InitializeRequest) (*protocol.InitializeResponse, error) {
	// We accept any client version today — future builds may compare
	// against a minimum-supported constant and return
	// protocol.ErrInvalidProtocolVersion when too old.
	//
	// Record the HITL kinds the client declared it can answer so the chat
	// service auto-denies any interrupt the client couldn't resolve rather
	// than parking into a deadlock (API.md §6.2). An empty / absent list is
	// treated permissively (surface all) so CLI / in-process callers that
	// don't negotiate keep working.
	if len(in.Capabilities.InterruptTypes) > 0 {
		// The kernel's SetInterruptKinds is wire-agnostic ([]string) — translate
		// the typed wire enum down at this delivery boundary (kernel never imports
		// the protocol package).
		kinds := make([]string, len(in.Capabilities.InterruptTypes))
		for i, t := range in.Capabilities.InterruptTypes {
			kinds[i] = string(t)
		}
		s.rt.Chat().SetInterruptKinds(kinds)
	}
	return &protocol.InitializeResponse{
		ProtocolVersion: protocol.ProtocolVersion, // server's truth — client falls back if needed
		ServerInfo:      s.serverInfo,
		Capabilities:    s.Capabilities(),
	}, nil
}

// Shutdown is a no-op — the in-process runtime has no per-connection
// "session" to tear down. The transport closes itself after this returns.
func (s *Server) Shutdown(_ context.Context, _ protocol.ShutdownRequest) error {
	return nil
}

// Ping is the liveness probe. Returns nil unless the runtime is in
// a degraded state (none of the current code paths set that).
func (s *Server) Ping(_ context.Context) error {
	return nil
}
