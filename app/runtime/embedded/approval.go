package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// GetApprovalMode returns the Runtime approval mode.
func (r *Runtime) GetApprovalMode(ctx context.Context, options CallOptions) (*protocol.ApprovalModeResult, error) {
	return invoke[struct{}, *protocol.ApprovalModeResult](ctx, r, "approval.getMode", struct{}{}, callOptions(options))
}

// SetApprovalMode changes the Runtime approval mode.
func (r *Runtime) SetApprovalMode(ctx context.Context, request protocol.SetApprovalModeRequest, options CommandOptions) (*protocol.ApprovalModeResult, error) {
	return invoke[protocol.SetApprovalModeRequest, *protocol.ApprovalModeResult](ctx, r, "approval.setMode", request, commandOptions(options))
}

// ListApprovalRules returns remembered approval rules.
func (r *Runtime) ListApprovalRules(ctx context.Context, request protocol.ListApprovalRulesRequest, options CallOptions) (*protocol.ListApprovalRulesResult, error) {
	return invoke[protocol.ListApprovalRulesRequest, *protocol.ListApprovalRulesResult](ctx, r, "approval.listRules", request, callOptions(options))
}

// ForgetApprovalRule removes one remembered approval rule.
func (r *Runtime) ForgetApprovalRule(ctx context.Context, request protocol.ForgetApprovalRuleRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "approval.forgetRule", request, commandOptions(options))
}
