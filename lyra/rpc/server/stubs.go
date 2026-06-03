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

func (i *Server) ListBackground(_ context.Context) ([]protocol.BackgroundTask, error) {
	return []protocol.BackgroundTask{}, nil
}

func (i *Server) SubscribeBackground(_ context.Context, _ string) (<-chan protocol.BackgroundTask, error) {
	return nil, notImpl("background.subscribe")
}

func (i *Server) CancelBackground(_ context.Context, _ string) error {
	return notImpl("background.cancel")
}

// ─── Feedback ───────────────────────────────────────────────────────
//
// The Runtime doesn't persist feedback yet; accept and drop so the
// frontend's UX flow can be exercised end-to-end (rather than erroring).
func (i *Server) CreateFeedback(_ context.Context, _ protocol.FeedbackRequest) error {
	return nil
}
