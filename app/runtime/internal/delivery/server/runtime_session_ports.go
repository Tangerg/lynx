package server

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"

	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

type sessionCatalogAccess interface {
	ListSessions(ctx context.Context) ([]sessionsvc.Session, error)
	SessionByID(ctx context.Context, id string) (sessionsvc.Session, error)
}

type sessionCreationAccess interface {
	CreateSession(ctx context.Context, title, cwd string) (sessionsvc.Session, error)
}

type sessionDeletionAccess interface {
	DeleteSession(ctx context.Context, id string) error
}

type sessionUpdateAccess interface {
	UpdateSession(ctx context.Context, id string, patch sessionsvc.Patch) (sessionsvc.Session, error)
}

type sessionDefaultModelAccess interface {
	DefaultModel() string
}

type transcriptContentAccess interface {
	ListTranscript(ctx context.Context, sessionID string) ([]transcript.Item, []transcript.Run, error)
}

type transcriptRunAccess interface {
	ListTranscriptRuns(ctx context.Context, sessionID string) ([]transcript.Run, error)
}

type historyAccess interface {
	ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error)
}
