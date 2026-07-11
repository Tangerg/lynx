package server

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"

	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// sessionUseCases is the session-facing slice of the application boundary.
// The server as a whole needs this cohesive context; individual handler methods
// still only call the operation they translate to wire.
type sessionUseCases interface {
	ListSessions(ctx context.Context) ([]sessionsvc.Session, error)
	SessionByID(ctx context.Context, id string) (sessionsvc.Session, error)
	CreateSession(ctx context.Context, title, cwd string) (sessionsvc.Session, error)
	UpdateSession(ctx context.Context, id string, patch sessionsvc.Patch) (sessionsvc.Session, error)
	DefaultModel() string
	ListTranscript(ctx context.Context, sessionID string) ([]transcript.Item, []transcript.Run, error)
	ListTranscriptRuns(ctx context.Context, sessionID string) ([]transcript.Run, error)
	ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error)
}
