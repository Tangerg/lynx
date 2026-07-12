package bootstrap

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel/trace"

	adapterhooks "github.com/Tangerg/lynx/app/runtime/internal/adapter/hooks"
)

// HookTrust reports whether a project root may run user lifecycle hooks.
type HookTrust interface {
	IsTrusted(ctx context.Context, projectRoot string) (bool, error)
}

// NewHookResolver builds the runtime hook resolver from process-local user config.
func NewHookResolver(trust HookTrust) HookResolver {
	userHome, _ := os.UserHomeDir()
	return adapterhooks.NewResolver(userHome,
		func(ctx context.Context, projectRoot string) bool {
			ok, _ := trust.IsTrusted(ctx, projectRoot)
			return ok
		},
		func(ctx context.Context, source string, err error) {
			trace.SpanFromContext(ctx).RecordError(fmt.Errorf("hook %s: %w", source, err))
		},
	)
}
