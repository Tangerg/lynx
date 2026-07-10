package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// Discover handles runtime.discover. It is a stateless capability query, not a
// lifecycle transition; business methods can run without calling it first.
func (s *Server) Discover(_ context.Context) (*protocol.DiscoverResponse, error) {
	return &protocol.DiscoverResponse{
		ProtocolVersion: protocol.ProtocolVersion,
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
