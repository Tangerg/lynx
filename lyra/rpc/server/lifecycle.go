package server

import (
	"context"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// Initialize handles runtime.initialize. Version negotiation
// follows the MCP rules (API.md §2.2): when the client's requested
// version differs from ours, we return the version WE support and
// let the client decide whether to fall back / disconnect.
func (i *Server) Initialize(_ context.Context, in protocol.InitializeRequest) (*protocol.InitializeResponse, error) {
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
		i.rt.Chat().SetInterruptKinds(in.Capabilities.InterruptTypes)
	}
	return &protocol.InitializeResponse{
		ProtocolVersion: protocol.ProtocolVersion, // server's truth — client falls back if needed
		ServerInfo:      i.serverInfo,
		Capabilities:    i.Capabilities(),
	}, nil
}

// Shutdown is best-effort — the in-process runtime doesn't have a
// per-connection "session" to tear down, so we treat shutdown as a
// no-op except to record it for telemetry. The transport closes
// itself after this returns.
func (i *Server) Shutdown(_ context.Context, _ protocol.ShutdownRequest) error {
	return nil
}

// Ping is the liveness probe. Returns nil unless the runtime is in
// a degraded state (none of the current code paths set that).
func (i *Server) Ping(_ context.Context) error {
	return nil
}
