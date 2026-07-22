package bootstrap

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
)

// goalMutationGuardRef late-binds the Goal driver into the session lifecycle
// coordinator. The coordinator is built before the Goal driver, while the
// driver depends on the run coordinator that itself depends on sessions.
type goalMutationGuardRef struct{ d *goals.Driver }

func (r *goalMutationGuardRef) WithSessionMutation(ctx context.Context, sessionIDs []string, apply func(context.Context) error) error {
	if r.d == nil {
		return apply(ctx)
	}
	return r.d.WithSessionMutation(ctx, sessionIDs, apply)
}
