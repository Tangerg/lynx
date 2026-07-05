package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// ListSessions returns every user-facing session, newest-updated first.
func (r *Runtime) ListSessions(ctx context.Context) ([]session.Session, error) {
	return r.session.List(ctx)
}

// GetSession returns one saved session by id.
func (r *Runtime) GetSession(ctx context.Context, id string) (session.Session, error) {
	return r.session.Get(ctx, id)
}

// CreateSession starts a fresh session in cwd.
func (r *Runtime) CreateSession(ctx context.Context, title, cwd string) (session.Session, error) {
	return r.session.Create(ctx, title, cwd)
}

// RenameSession changes a session's title.
func (r *Runtime) RenameSession(ctx context.Context, id, title string) error {
	return r.session.Rename(ctx, id, title)
}

// SetSessionModel records the model a session last ran against.
func (r *Runtime) SetSessionModel(ctx context.Context, id, model string) error {
	return r.session.SetModel(ctx, id, model)
}

// SetSessionCwd relocates a session's working-directory identity.
func (r *Runtime) SetSessionCwd(ctx context.Context, id, cwd string) error {
	return r.session.SetCwd(ctx, id, cwd)
}

// SetSessionMetadata full-replaces a session's free-form metadata.
func (r *Runtime) SetSessionMetadata(ctx context.Context, id string, meta map[string]any) error {
	return r.session.SetMetadata(ctx, id, meta)
}

// SetSessionFavorite pins or unpins a session.
func (r *Runtime) SetSessionFavorite(ctx context.Context, id string, favorite bool) error {
	return r.session.SetFavorite(ctx, id, favorite)
}
