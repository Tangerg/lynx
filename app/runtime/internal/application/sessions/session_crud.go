package sessions

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
)

// List returns every user-facing session, newest-updated first.
func (c *Coordinator) List(ctx context.Context) ([]session.Session, error) {
	return c.s.Session().List(ctx)
}

// Get returns one saved session by id.
func (c *Coordinator) Get(ctx context.Context, id string) (session.Session, error) {
	return c.s.Session().Get(ctx, id)
}

// Create starts a fresh session in cwd, resolving cwd to an existing directory
// (an unavailable cwd is [session.ErrCwdUnavailable]).
func (c *Coordinator) Create(ctx context.Context, title, cwd string) (session.Session, error) {
	cwd, err := resolveSessionCwd(cwd)
	if err != nil {
		return session.Session{}, err
	}
	return c.s.Session().Create(ctx, title, cwd)
}

// Update applies a session edit and returns the updated aggregate. Title (rename),
// model, cwd (relocate), metadata (full replace) and favorite are each optional;
// nil fields are left alone. The whole patch commits as one transaction so a
// mid-sequence failure leaves the session unmodified.
func (c *Coordinator) Update(ctx context.Context, id string, patch session.Patch) (session.Session, error) {
	patch, err := patch.Normalize()
	if err != nil {
		return session.Session{}, err
	}
	if patch.Cwd != nil {
		cwd, err := resolveSessionCwd(*patch.Cwd)
		if err != nil {
			return session.Session{}, err
		}
		patch.Cwd = &cwd
	}

	var updated session.Session
	err = c.s.RunInTx(ctx, func(tx context.Context) error {
		store := c.s.Session()
		if patch.Title != nil {
			if err := store.Rename(tx, id, *patch.Title); err != nil {
				return err
			}
		}
		if patch.Model != nil {
			if err := store.SetModel(tx, id, *patch.Model); err != nil {
				return err
			}
		}
		if patch.Cwd != nil {
			if err := store.SetCwd(tx, id, *patch.Cwd); err != nil {
				return err
			}
		}
		if patch.Metadata != nil {
			if err := store.SetMetadata(tx, id, *patch.Metadata); err != nil {
				return err
			}
		}
		if patch.Favorite != nil {
			if err := store.SetFavorite(tx, id, *patch.Favorite); err != nil {
				return err
			}
		}
		var err error
		updated, err = store.Get(tx, id)
		return err
	})
	if err != nil {
		return session.Session{}, err
	}
	return updated, nil
}

// resolveSessionCwd canonicalizes cwd and requires it to be an existing
// directory, joining [session.ErrCwdUnavailable] on failure.
func resolveSessionCwd(cwd string) (string, error) {
	resolved, err := worktree.ResolveExistingDir(cwd)
	if err != nil {
		return "", errors.Join(session.ErrCwdUnavailable, err)
	}
	return resolved, nil
}
