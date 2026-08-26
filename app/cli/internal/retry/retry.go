// Package retry owns transport-agnostic waiting and exponential backoff.
package retry

import (
	"context"
	"time"
)

// Backoff is an unbounded retry schedule whose delay has a finite ceiling.
// The caller's context, rather than an attempt budget, decides its lifetime.
type Backoff struct {
	Base    time.Duration
	Maximum time.Duration
}

func (b Backoff) Delay(failure int) time.Duration {
	base := max(b.Base, 0)
	maximum := max(b.Maximum, base)
	delay := base
	for range max(failure-1, 0) {
		if delay >= maximum/2 {
			return maximum
		}
		delay *= 2
	}
	return min(delay, maximum)
}

func Wait(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return context.Cause(ctx)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}
