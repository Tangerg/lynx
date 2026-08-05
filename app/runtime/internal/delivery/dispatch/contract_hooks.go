package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerHooks(r *Registry) {
	Query(r, MethodMeta{
		Name:      "hooks.list",
		Errors:    []string{protocol.ErrWorkspaceUnavailable.Error()},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.ListHooksRequest) (*protocol.HooksListResult, error) {
		return d.api.ListHooks(ctx, in)
	})

	CommandAck(r, MethodMeta{
		Name:      "hooks.setTrust",
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.SetHookTrustRequest) error {
		return d.api.SetHookTrust(ctx, in)
	})
}
