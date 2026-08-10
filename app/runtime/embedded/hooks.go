package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListHooks returns workspace hook discovery and trust state.
func (r *Runtime) ListHooks(ctx context.Context, request protocol.ListHooksRequest, options CallOptions) (*protocol.HooksListResult, error) {
	return invoke[protocol.ListHooksRequest, *protocol.HooksListResult](ctx, r, "hooks.list", request, callOptions(options))
}

// SetHookTrust changes trust for a workspace hook configuration.
func (r *Runtime) SetHookTrust(ctx context.Context, request protocol.SetHookTrustRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "hooks.setTrust", request, commandOptions(options))
}
