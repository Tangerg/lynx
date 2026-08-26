// Package teardown owns deadline-aware resource teardown for close operations
// that may not honor context cancellation.
package teardown

import (
	"context"
	"errors"
)

// Step is one terminal resource action. Sequence owns execution, ordering and
// joining; Step deliberately carries no second lifecycle state.
type Step struct {
	action func(context.Context) error
}

// Terminal returns a teardown step whose action reaching a return statement is
// the resource's final state. Its error is a shutdown diagnostic, not evidence
// that replaying the same one-shot Close can make further progress.
func Terminal(action func(context.Context) error) *Step {
	return &Step{action: action}
}

func (s *Step) run(ctx context.Context) error {
	if s == nil || s.action == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("teardown: context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.action(ctx)
}
