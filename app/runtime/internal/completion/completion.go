// Package completion owns the shared join rule for authoritative completion
// boundaries.
package completion

import (
	"context"
	"errors"
)

// Wait joins done or returns the caller's context error. Completion has
// precedence when it is already observable, including when cancellation and
// completion become ready together.
func Wait(ctx context.Context, done <-chan struct{}) error {
	if done == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("completion: wait context is required")
	}
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		select {
		case <-done:
			return nil
		default:
			return ctx.Err()
		}
	}
}
