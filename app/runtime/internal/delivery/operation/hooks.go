package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerHooks(registry *Registry) {
	Query(registry, MethodMeta{
		Name:      "hooks.list",
		Errors:    []string{protocol.ErrWorkspaceUnavailable.Error()},
		Stability: stable,
	}, func(service interface {
		ListHooks(context.Context, protocol.ListHooksRequest) (*protocol.HooksListResult, error)
	}, ctx context.Context, request protocol.ListHooksRequest) (*protocol.HooksListResult, error) {
		return service.ListHooks(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name:      "hooks.setTrust",
		Stability: stable,
	}, func(service interface {
		SetHookTrust(context.Context, protocol.SetHookTrustRequest) error
	}, ctx context.Context, request protocol.SetHookTrustRequest) error {
		return service.SetHookTrust(ctx, request)
	})
}
