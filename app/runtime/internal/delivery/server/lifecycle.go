package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// Discover handles runtime.discover. It is a stateless capability query, not a
// lifecycle transition; business methods can run without calling it first.
func (s *Server) Discover(_ context.Context) (*protocol.DiscoverResponse, error) {
	return &protocol.DiscoverResponse{
		ProtocolVersion: protocol.ProtocolVersion,
		ServerInfo:      s.serverInfo,
		Capabilities:    s.capabilities(),
	}, nil
}
