package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerHooks(registry *Registry) {
	Query(registry, MethodMeta{
		Name:      "hooks.list",
		Errors:    []string{protocol.ErrWorkspaceUnavailable.Error()},
		Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.ListHooksRequest) (*protocol.HooksListResult, error) {
		return router.api.ListHooks(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name:      "hooks.setTrust",
		Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.SetHookTrustRequest) error {
		return router.api.SetHookTrust(ctx, request)
	})
}
