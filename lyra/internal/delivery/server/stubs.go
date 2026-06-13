package server

import (
	"context"

	"github.com/Tangerg/lynx/lyra/internal/delivery/protocol"
)

// Attachments / Feedback (API.md §7.7) — surfaces with no engine backing
// yet, honestly gated off via notImpl, matching the capability flags
// advertised at initialize.

// ─── Attachments ────────────────────────────────────────────────────

func (s *Server) CreateUploadURL(_ context.Context, _ protocol.CreateUploadURLRequest) (*protocol.CreateUploadURLResponse, error) {
	return nil, notImpl("attachments.createUploadUrl")
}

func (s *Server) GetAttachment(_ context.Context, _ string) (*protocol.Attachment, error) {
	return nil, notImpl("attachments.get")
}

func (s *Server) DeleteAttachment(_ context.Context, _ string) error {
	return notImpl("attachments.delete")
}

// ─── Feedback ───────────────────────────────────────────────────────
//
// feedback.create is ungated (API.md §7.7) and has no readback method, so
// "accepted" is a truthful ack — the contract never promises durable
// storage. The Runtime doesn't retain feedback yet (write-only-never-read
// data isn't worth a store); accept it. Add a sink (OTel / store) when a
// real consumer exists.
func (s *Server) CreateFeedback(_ context.Context, _ protocol.FeedbackRequest) error {
	return nil
}
