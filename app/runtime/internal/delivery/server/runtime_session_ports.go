package server

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"

	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

type sessionAccess interface {
	ListSessions(ctx context.Context) ([]sessionsvc.Session, error)
	SessionByID(ctx context.Context, id string) (sessionsvc.Session, error)
	CreateSession(ctx context.Context, title, cwd string) (sessionsvc.Session, error)
	DeleteSession(ctx context.Context, id string) error
	UpdateSession(ctx context.Context, id string, patch sessionsvc.Patch) (sessionsvc.Session, error)
	DefaultModel() string
}

type transcriptAccess interface {
	ListTranscript(ctx context.Context, sessionID string) ([]transcript.Item, []transcript.Run, error)
	ListTranscriptRuns(ctx context.Context, sessionID string) ([]transcript.Run, error)
}

type historyAccess interface {
	ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error)
}
