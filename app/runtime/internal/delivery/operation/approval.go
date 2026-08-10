package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerApproval(registry *Registry) {
	Query(registry, MethodMeta{Name: "approval.getMode", Stability: stable},
		func(service Service, ctx context.Context, _ struct{}) (*protocol.ApprovalModeResult, error) {
			return service.GetApprovalMode(ctx)
		})

	Command(registry, MethodMeta{Name: "approval.setMode", Stability: stable},
		func(service Service, ctx context.Context, request protocol.SetApprovalModeRequest) (*protocol.ApprovalModeResult, error) {
			return service.SetApprovalMode(ctx, request)
		})

	Query(registry, MethodMeta{Name: "approval.listRules", Stability: stable},
		func(service Service, ctx context.Context, request protocol.ListApprovalRulesRequest) (*protocol.ListApprovalRulesResult, error) {
			return service.ListApprovalRules(ctx, request)
		})

	CommandAck(registry, MethodMeta{Name: "approval.forgetRule", Stability: stable},
		func(service Service, ctx context.Context, request protocol.ForgetApprovalRuleRequest) error {
			return service.ForgetApprovalRule(ctx, request)
		})
}
