package bootstrap

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/trace"

	adapterhooks "github.com/Tangerg/lynx/app/runtime/internal/adapter/hooks"
)

// HookTrust reports whether a project root may run user lifecycle hooks.
type HookTrust interface {
	IsTrusted(ctx context.Context, projectRoot string) (bool, error)
}

// NewHookResolver builds the runtime hook resolver from the composition root's
// user-home snapshot and the durable project trust policy.
func NewHookResolver(userHome string, trust HookTrust) *adapterhooks.Resolver {
	return adapterhooks.NewResolver(userHome,
		func(ctx context.Context, projectRoot string) (bool, error) {
			if trust == nil {
				return false, nil
			}
			ok, err := trust.IsTrusted(ctx, projectRoot)
			if err != nil {
				return false, fmt.Errorf("hooks: read trust for project %q: %w", projectRoot, err)
			}
			return ok, nil
		},
		func(ctx context.Context, source string, err error) {
			trace.SpanFromContext(ctx).RecordError(fmt.Errorf("hook %s: %w", source, err))
		},
	)
}
