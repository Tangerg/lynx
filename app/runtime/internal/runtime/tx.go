package runtime

import "context"

// runInTx runs fn inside one storage transaction (commit on success, rollback
// on error), so a multi-step write-set across the domain services commits
// atomically. Falls back to running fn directly when no transactor is wired (a
// non-sqlite / test runtime) -- correct but without all-or-nothing.
func (r *Runtime) runInTx(ctx context.Context, fn func(context.Context) error) error {
	if r == nil || r.transactor == nil {
		return fn(ctx)
	}
	return r.transactor(ctx, fn)
}
