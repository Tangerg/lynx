package operation

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/protocol"
)

const (
	ApprovalGetMode    Name = "approval.getMode"
	ApprovalSetMode    Name = "approval.setMode"
	ApprovalListRules  Name = "approval.listRules"
	ApprovalForgetRule Name = "approval.forgetRule"
)

func registerApproval(registry *Registry) {
	registry.Query(MethodMeta{Name: ApprovalGetMode},
		func(service interface {
			GetApprovalMode(context.Context) (*protocol.ApprovalModeResult, error)
		}, ctx context.Context, _ struct{}) (*protocol.ApprovalModeResult, error) {
			return service.GetApprovalMode(ctx)
		})

	registry.Command(MethodMeta{Name: ApprovalSetMode},
		func(service interface {
			SetApprovalMode(context.Context, protocol.SetApprovalModeRequest) (*protocol.ApprovalModeResult, error)
		}, ctx context.Context, request protocol.SetApprovalModeRequest) (*protocol.ApprovalModeResult, error) {
			return service.SetApprovalMode(ctx, request)
		})

	registry.Query(MethodMeta{Name: ApprovalListRules},
		func(service interface {
			ListApprovalRules(context.Context, protocol.ListApprovalRulesRequest) (*protocol.ListApprovalRulesResult, error)
		}, ctx context.Context, request protocol.ListApprovalRulesRequest) (*protocol.ListApprovalRulesResult, error) {
			return service.ListApprovalRules(ctx, request)
		})

	registry.CommandAck(MethodMeta{Name: ApprovalForgetRule},
		func(service interface {
			ForgetApprovalRule(context.Context, protocol.ForgetApprovalRuleRequest) error
		}, ctx context.Context, request protocol.ForgetApprovalRuleRequest) error {
			return service.ForgetApprovalRule(ctx, request)
		})
}
