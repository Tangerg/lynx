package embedded

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// GetApprovalMode returns the Runtime approval mode.
func (r *Runtime) GetApprovalMode(ctx context.Context, options CallOptions) (*protocol.ApprovalModeResult, error) {
	return r.invoke[struct{}, *protocol.ApprovalModeResult](ctx, operation.ApprovalGetMode, struct{}{}, callOptions(options))
}

// SetApprovalMode changes the Runtime approval mode.
func (r *Runtime) SetApprovalMode(ctx context.Context, request protocol.SetApprovalModeRequest, options CommandOptions) (*protocol.ApprovalModeResult, error) {
	return r.invoke[protocol.SetApprovalModeRequest, *protocol.ApprovalModeResult](ctx, operation.ApprovalSetMode, request, commandOptions(options))
}

// ListApprovalRules returns remembered approval rules.
func (r *Runtime) ListApprovalRules(ctx context.Context, request protocol.ListApprovalRulesRequest, options CallOptions) (*protocol.ListApprovalRulesResult, error) {
	return r.invoke[protocol.ListApprovalRulesRequest, *protocol.ListApprovalRulesResult](ctx, operation.ApprovalListRules, request, callOptions(options))
}

// ForgetApprovalRule removes one remembered approval rule.
func (r *Runtime) ForgetApprovalRule(ctx context.Context, request protocol.ForgetApprovalRuleRequest, options CommandOptions) error {
	return r.invokeAck(ctx, operation.ApprovalForgetRule, request, commandOptions(options))
}
