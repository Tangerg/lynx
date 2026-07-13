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

// SetModel records the model explicitly selected for a run. It is the narrow
// mutation consumed by application/runs; callers that need the full editable
// session surface use Update.
func (c *Coordinator) SetModel(ctx context.Context, id, model string) error {
	_, err := c.s.Session().Patch(ctx, id, session.Patch{Model: &model})
	return err
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

	return c.s.Session().Patch(ctx, id, patch)
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
