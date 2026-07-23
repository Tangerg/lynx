// Package approvals owns the runtime tool-permission use cases: the approval
// stance (mode) and the persisted per-session/project/global approval rules.
package approvals

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// SessionLookup resolves a session so rule listing can scope rules to the
// session's project directory. The session store satisfies it.
type SessionLookup interface {
	Get(ctx context.Context, id string) (session.Session, error)
}

// Policy is the approval-management use case's view of runtime policy. Tool
// call evaluation has a different, narrower consumer interface in turn.
type Policy interface {
	Mode(ctx context.Context) (approval.Mode, error)
	SetMode(ctx context.Context, mode approval.Mode) error
	Rules(ctx context.Context, sessionID, projectDir string) ([]approval.Rule, error)
	Forget(ctx context.Context, id string) error
}

// Coordinator drives the tool-permission stance + approval-rule use cases.
type Coordinator struct {
	policy   Policy
	sessions SessionLookup
}

// New returns a Coordinator over the approval policy + the session lookup its
// rule scoping reads.
func New(policy Policy, sessions SessionLookup) *Coordinator {
	return &Coordinator{policy: policy, sessions: sessions}
}

// Mode returns the current runtime tool-permission stance.
func (c *Coordinator) Mode(ctx context.Context) (approval.Mode, error) {
	return c.policy.Mode(ctx)
}

// SetMode changes the runtime tool-permission stance.
func (c *Coordinator) SetMode(ctx context.Context, mode approval.Mode) error {
	return c.policy.SetMode(ctx, mode)
}

// ListRules returns the rules visible from a session. Unknown sessions degrade to
// session/global lookup; storage failures are real errors.
func (c *Coordinator) ListRules(ctx context.Context, sessionID string) ([]approval.Rule, error) {
	cwd := ""
	if sessionID != "" {
		switch sess, err := c.sessions.Get(ctx, sessionID); {
		case err == nil:
			cwd = sess.Cwd
		case !errors.Is(err, session.ErrNotFound):
			return nil, err
		}
	}
	return c.policy.Rules(ctx, sessionID, cwd)
}

// ForgetRule removes one persisted approval rule by id.
func (c *Coordinator) ForgetRule(ctx context.Context, id string) error {
	return c.policy.Forget(ctx, id)
}
