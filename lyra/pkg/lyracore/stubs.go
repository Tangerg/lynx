package lyracore

import (
	"context"

	"github.com/Tangerg/lynx/lyra/pkg/coreapi"
)

// attachments.* — no upload backend yet. The protocol's binary
// carve-out (API.md §5.4) needs a PUT target, which only makes
// sense once the HTTP transport grows a real binary endpoint.

func (i *Server) CreateUploadURL(_ context.Context, _ coreapi.CreateUploadURLRequest) (*coreapi.CreateUploadURLResponse, error) {
	return nil, notImpl("attachments.createUploadUrl")
}

func (i *Server) DeleteAttachment(_ context.Context, _ string) error {
	return notImpl("attachments.delete")
}

// background.* — long-running task surface; nothing in the engine
// emits BackgroundTask state today.

func (i *Server) ListBackground(_ context.Context) ([]coreapi.BackgroundTask, error) {
	return []coreapi.BackgroundTask{}, nil
}

func (i *Server) StopBackground(_ context.Context, _ string) error {
	return notImpl("background.stop")
}

func (i *Server) SubscribeBackground(_ context.Context, _ string) (<-chan coreapi.BackgroundUpdate, error) {
	return nil, notImpl("background.subscribe")
}

// feedback.* — RLHF data collection. The Runtime doesn't persist
// feedback yet; accept and drop until storage is settled (rather
// than returning a hard not-implemented), so the frontend's UX flow
// can be tested end-to-end.

func (i *Server) SubmitFeedback(_ context.Context, _ coreapi.FeedbackRequest) error {
	return nil
}
