package capabilities

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// ApprovalMode returns the current runtime tool-permission stance.
func (c *Coordinator) ApprovalMode(ctx context.Context) (approval.Mode, error) {
	return c.approval.Mode(ctx)
}

// SetApprovalMode changes the runtime tool-permission stance.
func (c *Coordinator) SetApprovalMode(ctx context.Context, mode approval.Mode) error {
	return c.approval.SetMode(ctx, mode)
}

// ListApprovalRules returns the rules visible from a session. Unknown sessions
// degrade to session/global lookup; storage failures are real errors.
func (c *Coordinator) ListApprovalRules(ctx context.Context, sessionID string) ([]approval.Rule, error) {
	cwd := ""
	if sessionID != "" {
		sess, err := c.sessions.Get(ctx, sessionID)
		if err != nil {
			if !errors.Is(err, session.ErrNotFound) {
				return nil, err
			}
		} else {
			cwd = sess.Cwd
		}
	}
	return c.approval.Rules(ctx, sessionID, cwd)
}

// ForgetApprovalRule removes one persisted approval rule by id.
func (c *Coordinator) ForgetApprovalRule(ctx context.Context, id string) error {
	return c.approval.Forget(ctx, id)
}
