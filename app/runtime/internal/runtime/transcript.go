package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// ListTranscript returns a session's durable item history and run records.
func (r *Runtime) ListTranscript(ctx context.Context, sessionID string) ([]transcript.Item, []transcript.Run, error) {
	if r.transcript == nil {
		return nil, nil, nil
	}
	return r.transcript.List(ctx, sessionID)
}

// ListTranscriptRuns returns a session's durable run records.
func (r *Runtime) ListTranscriptRuns(ctx context.Context, sessionID string) ([]transcript.Run, error) {
	if r.transcript == nil {
		return nil, nil
	}
	return r.transcript.ListRuns(ctx, sessionID)
}
