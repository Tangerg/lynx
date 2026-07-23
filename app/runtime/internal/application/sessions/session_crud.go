package sessions

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// CwdResolver is the filesystem boundary needed when a session is created or
// relocated. Session.Cwd is canonical after admission, so downstream use cases
// treat it as an invariant instead of repeatedly touching the filesystem.
type CwdResolver interface {
	ResolveExistingDir(path string) (string, error)
	Inspect(path string) (session.WorkspaceIdentity, error)
}

// ModelDefaults supplies the runtime's configured fallback model. Model
// selection is application policy: a persisted session only records an
// explicit choice, while the read model resolves the effective value.
type ModelDefaults interface {
	DefaultModel() string
}

// List returns every user-facing session, newest-updated first.
func (c *Coordinator) List(ctx context.Context) ([]session.Session, error) {
	if c.sessions == nil {
		return nil, errors.New("sessions: session store is unavailable")
	}
	return c.sessions.List(ctx)
}

// Get returns one saved session by id.
func (c *Coordinator) Get(ctx context.Context, id string) (session.Session, error) {
	if c.sessions == nil {
		return session.Session{}, errors.New("sessions: session store is unavailable")
	}
	return c.sessions.Get(ctx, id)
}

// InspectWorkspace resolves the live filesystem projection of an admitted cwd.
// Missing directories are represented in the returned value; unexpected
// filesystem failures remain errors so delivery never silently lies.
func (c *Coordinator) InspectWorkspace(cwd string) (session.WorkspaceIdentity, error) {
	if c.paths == nil {
		return session.WorkspaceIdentity{}, errors.New("sessions: workspace inspector is unavailable")
	}
	return c.paths.Inspect(cwd)
}

// Create starts a fresh session in cwd, resolving cwd to an existing directory
// (an unavailable cwd is [session.ErrCwdUnavailable]).
func (c *Coordinator) Create(ctx context.Context, title, cwd string) (session.Session, error) {
	cwd, err := c.resolveSessionCwd(cwd)
	if err != nil {
		return session.Session{}, err
	}
	if c.sessions == nil {
		return session.Session{}, errors.New("sessions: session store is unavailable")
	}
	return c.sessions.Create(ctx, title, cwd)
}

// SetModel records the model explicitly selected for a run. It is the narrow
// mutation consumed by application/runs; callers that need the full editable
// session surface use Update.
func (c *Coordinator) SetModel(ctx context.Context, id, model string) error {
	if c.sessions == nil {
		return errors.New("sessions: session store is unavailable")
	}
	_, err := c.sessions.Patch(ctx, id, session.Patch{Model: &model})
	return err
}

// Update applies a session edit and returns the updated aggregate. Title (rename),
// model, cwd (relocate), and favorite are each optional;
// nil fields are left alone. The whole patch commits as one transaction so a
// mid-sequence failure leaves the session unmodified.
func (c *Coordinator) Update(ctx context.Context, id string, patch session.Patch) (session.Session, error) {
	patch, err := patch.Normalize()
	if err != nil {
		return session.Session{}, err
	}
	if patch.Cwd != nil {
		cwd, err := c.resolveSessionCwd(*patch.Cwd)
		if err != nil {
			return session.Session{}, err
		}
		patch.Cwd = &cwd
		admission, err := c.ClaimMutationSlot(id)
		if err != nil {
			return session.Session{}, err
		}
		defer admission.Release()
	}

	if c.sessions == nil {
		return session.Session{}, errors.New("sessions: session store is unavailable")
	}
	return c.sessions.Patch(ctx, id, patch)
}

// resolveSessionCwd canonicalizes cwd and requires it to be an existing
// directory, joining [session.ErrCwdUnavailable] on failure.
func (c *Coordinator) resolveSessionCwd(cwd string) (string, error) {
	if c.paths == nil {
		return "", errors.Join(session.ErrCwdUnavailable, errors.New("sessions: cwd resolver is unavailable"))
	}
	resolved, err := c.paths.ResolveExistingDir(cwd)
	if err != nil {
		return "", errors.Join(session.ErrCwdUnavailable, err)
	}
	return resolved, nil
}
