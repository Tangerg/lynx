package sessions

import (
	"context"
	"errors"
	"fmt"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// CWDResolver is the filesystem boundary needed when a Session is created or
// relocated. Session.CWD is canonical after admission, so downstream use cases
// treat it as an invariant instead of repeatedly touching the filesystem.
type CWDResolver interface {
	ResolveExistingDir(path string) (string, error)
	Inspect(path string) (workspaceapp.Resolved, error)
}

// List returns every user-facing Session, newest-updated first.
func (c *Coordinator) List(ctx context.Context) ([]session.Session, error) {
	return c.sessions.List(ctx)
}

// Get returns one saved Session by identity.
func (c *Coordinator) Get(ctx context.Context, id string) (session.Session, error) {
	return c.sessions.Get(ctx, id)
}

// InspectWorkspace resolves the live filesystem projection of an admitted cwd.
// Missing directories are represented in the returned value; unexpected
// filesystem failures remain errors so callers never receive a fabricated
// workspace identity.
func (c *Coordinator) InspectWorkspace(cwd string) (workspaceapp.Resolved, error) {
	return c.paths.Inspect(cwd)
}

// Create starts and persists a fresh root Session in an admitted workspace.
func (c *Coordinator) Create(ctx context.Context, title, cwd string) (session.Session, error) {
	cwd, err := c.resolveSessionCWD(cwd)
	if err != nil {
		return session.Session{}, err
	}
	created, err := session.New(session.Draft{
		ID: c.newID(), Title: title, CWD: cwd, StartedAt: c.now(),
	})
	if err != nil {
		return session.Session{}, err
	}
	if err := c.sessions.Insert(ctx, created); err != nil {
		return session.Session{}, err
	}
	c.publishSessionMoved(created.ID())
	return created, nil
}

// PrepareScheduled resolves a schedule-owned Session without writing it. When
// the identity already exists, it returns that durable aggregate and no initial
// value. Otherwise it returns one initial aggregate for the Run opening write-set.
// This makes a retried occurrence reuse its Session without asking persistence
// to derive or normalize product state.
func (c *Coordinator) PrepareScheduled(
	ctx context.Context,
	id, title, cwd, model string,
) (current session.Session, initial *session.Session, err error) {
	existing, err := c.sessions.Get(ctx, id)
	if err == nil {
		return existing, nil, nil
	}
	if !errors.Is(err, session.ErrNotFound) {
		return session.Session{}, nil, err
	}
	cwd, err = c.resolveSessionCWD(cwd)
	if err != nil {
		return session.Session{}, nil, err
	}
	created, err := session.New(session.Draft{
		ID: id, Title: title, CWD: cwd, Model: model, StartedAt: c.now(),
	})
	if err != nil {
		return session.Session{}, nil, err
	}
	return created, &created, nil
}

// Update applies one complete Session edit. External workspace admission and
// process-local mutation claims occur before Domain behavior; persistence only
// saves the resulting aggregate with CAS.
func (c *Coordinator) Update(ctx context.Context, id string, patch session.Patch) (session.Session, error) {
	if patch.CWD != nil {
		cwd, err := c.resolveSessionCWD(*patch.CWD)
		if err != nil {
			return session.Session{}, err
		}
		patch.CWD = &cwd
	}
	if patch.CWD != nil || patch.Isolated != nil {
		admission, err := c.ClaimIdleSession(ctx, id)
		if err != nil {
			return session.Session{}, err
		}
		defer admission.Release()
	}
	current, err := c.sessions.Get(ctx, id)
	if err != nil {
		return session.Session{}, err
	}
	updated, changed, err := current.Apply(patch, c.now())
	if err != nil {
		return session.Session{}, err
	}
	if !changed {
		return current, nil
	}
	if err := c.sessions.Save(ctx, current.Revision(), updated); err != nil {
		return session.Session{}, err
	}
	c.publishSessionMoved(id)
	return updated, nil
}

// NeedsGeneratedTitle reports whether title generation can still contribute a
// value. The later write rechecks this condition, so a user rename between the
// query and generated result always wins.
func (c *Coordinator) NeedsGeneratedTitle(ctx context.Context, id string) (bool, error) {
	value, err := c.Get(ctx, id)
	if err != nil {
		return false, err
	}
	return value.Title() == "", nil
}

// ApplyGeneratedTitle installs a generated title using Domain first-writer
// semantics. Concurrent Session edits are retried from their committed value;
// once a user title exists, the generated title becomes a successful no-op.
func (c *Coordinator) ApplyGeneratedTitle(ctx context.Context, id, title string) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := c.sessions.Get(ctx, id)
		if err != nil {
			return err
		}
		updated, changed, err := current.NameIfUntitled(title, c.now())
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		if err := c.sessions.Save(ctx, current.Revision(), updated); err == nil {
			c.publishSessionMoved(id)
			return nil
		} else if !errors.Is(err, session.ErrRevisionConflict) {
			return fmt.Errorf("sessions: save generated title: %w", err)
		}
	}
}

// resolveSessionCWD canonicalizes cwd and requires it to be an existing
// directory, returning the shared workspace-admission sentinel on failure.
func (c *Coordinator) resolveSessionCWD(cwd string) (string, error) {
	resolved, err := c.paths.ResolveExistingDir(cwd)
	if err != nil {
		return "", errors.Join(workspaceapp.ErrCWDUnavailable, err)
	}
	return resolved, nil
}
