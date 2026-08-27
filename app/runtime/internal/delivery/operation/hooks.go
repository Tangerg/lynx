package operation

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/protocol"
)

const (
	HooksList     Name = "hooks.list"
	HooksSetTrust Name = "hooks.setTrust"
)

func registerHooks(registry *Registry) {
	registry.Query(MethodMeta{
		Name:   HooksList,
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
	}, func(service interface {
		ListHooks(context.Context, protocol.ListHooksRequest) (*protocol.HooksListResult, error)
	}, ctx context.Context, request protocol.ListHooksRequest) (*protocol.HooksListResult, error) {
		return service.ListHooks(ctx, request)
	})

	registry.CommandAck(MethodMeta{
		Name: HooksSetTrust,
	}, func(service interface {
		SetHookTrust(context.Context, protocol.SetHookTrustRequest) error
	}, ctx context.Context, request protocol.SetHookTrustRequest) error {
		return service.SetHookTrust(ctx, request)
	})
}
