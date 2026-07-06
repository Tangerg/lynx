package runtime

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
)

// ListSessions returns every user-facing session, newest-updated first.
func (r *Runtime) ListSessions(ctx context.Context) ([]session.Session, error) {
	return r.session.List(ctx)
}

// SessionByID returns one saved session by id.
func (r *Runtime) SessionByID(ctx context.Context, id string) (session.Session, error) {
	return r.session.Get(ctx, id)
}

// CreateSession starts a fresh session in cwd.
func (r *Runtime) CreateSession(ctx context.Context, title, cwd string) (session.Session, error) {
	cwd, err := resolveSessionCwd(cwd)
	if err != nil {
		return session.Session{}, err
	}
	return r.session.Create(ctx, title, cwd)
}

// UpdateSession applies a session edit and returns the updated aggregate.
func (r *Runtime) UpdateSession(ctx context.Context, id string, patch session.Patch) (session.Session, error) {
	if patch.Title != nil {
		title := strings.TrimSpace(*patch.Title)
		if title == "" {
			return session.Session{}, session.ErrTitleRequired
		}
		patch.Title = &title
	}
	if patch.Cwd != nil {
		cwd, err := resolveSessionCwd(*patch.Cwd)
		if err != nil {
			return session.Session{}, err
		}
		patch.Cwd = &cwd
	}

	var updated session.Session
	err := r.runInTx(ctx, func(tx context.Context) error {
		if patch.Title != nil {
			if err := r.session.Rename(tx, id, *patch.Title); err != nil {
				return err
			}
		}
		if patch.Model != nil {
			if err := r.session.SetModel(tx, id, *patch.Model); err != nil {
				return err
			}
		}
		if patch.Cwd != nil {
			if err := r.session.SetCwd(tx, id, *patch.Cwd); err != nil {
				return err
			}
		}
		if patch.Metadata != nil {
			if err := r.session.SetMetadata(tx, id, *patch.Metadata); err != nil {
				return err
			}
		}
		if patch.Favorite != nil {
			if err := r.session.SetFavorite(tx, id, *patch.Favorite); err != nil {
				return err
			}
		}
		var err error
		updated, err = r.session.Get(tx, id)
		return err
	})
	if err != nil {
		return session.Session{}, err
	}
	return updated, nil
}

func resolveSessionCwd(cwd string) (string, error) {
	resolved, err := worktree.ResolveExistingDir(cwd)
	if err != nil {
		return "", errors.Join(session.ErrCwdUnavailable, err)
	}
	return resolved, nil
}
