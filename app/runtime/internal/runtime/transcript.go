package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

type transcriptContent interface {
	List(ctx context.Context, sessionID string) ([]transcript.Item, []transcript.Run, error)
}

type transcriptRuns interface {
	ListRuns(ctx context.Context, sessionID string) ([]transcript.Run, error)
}

// ListTranscript returns a session's durable item history and run records.
func (r *Runtime) ListTranscript(ctx context.Context, sessionID string) ([]transcript.Item, []transcript.Run, error) {
	return r.transcriptContent.List(ctx, sessionID)
}

// ListTranscriptRuns returns a session's durable run records.
func (r *Runtime) ListTranscriptRuns(ctx context.Context, sessionID string) ([]transcript.Run, error) {
	return r.transcriptRuns.ListRuns(ctx, sessionID)
}
