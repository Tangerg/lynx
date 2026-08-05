package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerApproval(r *Registry) {
	Query(r, MethodMeta{Name: "approval.getMode", Stability: stable},
		func(d *Router, ctx context.Context, _ struct{}) (*protocol.ApprovalModeResult, error) {
			return d.api.GetApprovalMode(ctx)
		})

	Command(r, MethodMeta{Name: "approval.setMode", Stability: stable},
		func(d *Router, ctx context.Context, in protocol.SetApprovalModeRequest) (*protocol.ApprovalModeResult, error) {
			return d.api.SetApprovalMode(ctx, in)
		})

	Query(r, MethodMeta{Name: "approval.listRules", Stability: stable},
		func(d *Router, ctx context.Context, in protocol.ListApprovalRulesRequest) (*protocol.ListApprovalRulesResult, error) {
			return d.api.ListApprovalRules(ctx, in)
		})

	CommandAck(r, MethodMeta{Name: "approval.forgetRule", Stability: stable},
		func(d *Router, ctx context.Context, in protocol.ForgetApprovalRuleRequest) error {
			return d.api.ForgetApprovalRule(ctx, in)
		})
}
