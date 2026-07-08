package runtime

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

type approvalModeReader interface {
	Mode(ctx context.Context) (approval.Mode, error)
}

type approvalModeWriter interface {
	SetMode(ctx context.Context, mode approval.Mode) error
}

type approvalRuleLister interface {
	Rules(ctx context.Context, sessionID, projectDir string) ([]approval.Rule, error)
}

type approvalRuleDeleter interface {
	Forget(ctx context.Context, id string) error
}

// ApprovalMode returns the current runtime tool-permission stance.
func (r *Runtime) ApprovalMode(ctx context.Context) (approval.Mode, error) {
	return r.approvalModeRead.Mode(ctx)
}

// SetApprovalMode changes the runtime tool-permission stance.
func (r *Runtime) SetApprovalMode(ctx context.Context, mode approval.Mode) error {
	return r.approvalModeMutation.SetMode(ctx, mode)
}

// ListApprovalRules returns the rules visible from a session. Unknown sessions
// degrade to session/global lookup; storage failures are real errors.
func (r *Runtime) ListApprovalRules(ctx context.Context, sessionID string) ([]approval.Rule, error) {
	cwd := ""
	if sessionID != "" {
		sess, err := r.sessionRead.Get(ctx, sessionID)
		if err != nil {
			if !errors.Is(err, session.ErrNotFound) {
				return nil, err
			}
		} else {
			cwd = sess.Cwd
		}
	}
	return r.approvalRuleList.Rules(ctx, sessionID, cwd)
}

// ForgetApprovalRule removes one persisted approval rule by id.
func (r *Runtime) ForgetApprovalRule(ctx context.Context, id string) error {
	return r.approvalRuleDeletion.Forget(ctx, id)
}
