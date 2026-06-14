package server

import (
	"context"

	"github.com/Tangerg/lynx/lyra/internal/delivery/protocol"
)

// feedback.create is ungated (API.md §7.7) and has no readback method, so
// "accepted" is a truthful ack — the contract never promises durable
// storage. The Runtime doesn't retain feedback yet (write-only-never-read
// data isn't worth a store); accept it. Add a sink (OTel / store) when a
// real consumer exists.
func (s *Server) CreateFeedback(_ context.Context, _ protocol.FeedbackRequest) error {
	return nil
}
