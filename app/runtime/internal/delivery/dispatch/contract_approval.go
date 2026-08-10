package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerApproval(registry *Registry) {
	Query(registry, MethodMeta{Name: "approval.getMode", Stability: stable},
		func(router *Router, ctx context.Context, _ struct{}) (*protocol.ApprovalModeResult, error) {
			return router.api.GetApprovalMode(ctx)
		})

	Command(registry, MethodMeta{Name: "approval.setMode", Stability: stable},
		func(router *Router, ctx context.Context, request protocol.SetApprovalModeRequest) (*protocol.ApprovalModeResult, error) {
			return router.api.SetApprovalMode(ctx, request)
		})

	Query(registry, MethodMeta{Name: "approval.listRules", Stability: stable},
		func(router *Router, ctx context.Context, request protocol.ListApprovalRulesRequest) (*protocol.ListApprovalRulesResult, error) {
			return router.api.ListApprovalRules(ctx, request)
		})

	CommandAck(registry, MethodMeta{Name: "approval.forgetRule", Stability: stable},
		func(router *Router, ctx context.Context, request protocol.ForgetApprovalRuleRequest) error {
			return router.api.ForgetApprovalRule(ctx, request)
		})
}
