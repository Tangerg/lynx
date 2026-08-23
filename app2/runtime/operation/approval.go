package operation

import (
	"context"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func registerApproval(registry *Registry) {
	Query(registry, MethodMeta{Name: "approval.getMode"},
		func(service interface {
			GetApprovalMode(context.Context) (*protocol.ApprovalModeResult, error)
		}, ctx context.Context, _ struct{}) (*protocol.ApprovalModeResult, error) {
			return service.GetApprovalMode(ctx)
		})

	Command(registry, MethodMeta{Name: "approval.setMode"},
		func(service interface {
			SetApprovalMode(context.Context, protocol.SetApprovalModeRequest) (*protocol.ApprovalModeResult, error)
		}, ctx context.Context, request protocol.SetApprovalModeRequest) (*protocol.ApprovalModeResult, error) {
			return service.SetApprovalMode(ctx, request)
		})

	Query(registry, MethodMeta{Name: "approval.listRules"},
		func(service interface {
			ListApprovalRules(context.Context, protocol.ListApprovalRulesRequest) (*protocol.ListApprovalRulesResult, error)
		}, ctx context.Context, request protocol.ListApprovalRulesRequest) (*protocol.ListApprovalRulesResult, error) {
			return service.ListApprovalRules(ctx, request)
		})

	CommandAck(registry, MethodMeta{Name: "approval.forgetRule"},
		func(service interface {
			ForgetApprovalRule(context.Context, protocol.ForgetApprovalRuleRequest) error
		}, ctx context.Context, request protocol.ForgetApprovalRuleRequest) error {
			return service.ForgetApprovalRule(ctx, request)
		})
}
