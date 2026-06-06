package server

import (
	"context"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// Attachments / Background / Feedback (API.md §7.7) — surfaces with no
// engine backing yet. List endpoints return empty (valid "nothing here"
// answers); the rest are honestly gated off via notImpl, matching the
// capability flags advertised at initialize.

// ─── Attachments ────────────────────────────────────────────────────

func (i *Server) CreateUploadURL(_ context.Context, _ protocol.CreateUploadURLRequest) (*protocol.CreateUploadURLResponse, error) {
	return nil, notImpl("attachments.createUploadUrl")
}

func (i *Server) GetAttachment(_ context.Context, _ string) (*protocol.Attachment, error) {
	return nil, notImpl("attachments.get")
}

func (i *Server) DeleteAttachment(_ context.Context, _ string) error {
	return notImpl("attachments.delete")
}

// ─── Background ─────────────────────────────────────────────────────

// ListBackground is gated off (features.background=false) — return
// capability_not_negotiated rather than a misleading empty list, matching
// background.subscribe / cancel (API.md §7.7).
func (i *Server) ListBackground(_ context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.BackgroundTask], error) {
	return nil, notImpl("background.list")
}

func (i *Server) SubscribeBackground(_ context.Context, _ string) (<-chan protocol.BackgroundTask, error) {
	return nil, notImpl("background.subscribe")
}

func (i *Server) CancelBackground(_ context.Context, _ string) error {
	return notImpl("background.cancel")
}

// ─── Feedback ───────────────────────────────────────────────────────
//
// feedback.create is ungated (API.md §7.7) and has no readback method, so
// "accepted" is a truthful ack — the contract never promises durable
// storage. The Runtime doesn't retain feedback yet (write-only-never-read
// data isn't worth a store); accept it. Add a sink (OTel / store) when a
// real consumer exists.
func (i *Server) CreateFeedback(_ context.Context, _ protocol.FeedbackRequest) error {
	return nil
}
