// Package reconnect owns transport retry policy shared by interactive and
// headless delivery adapters. It classifies symbolic agent-port errors, never error
// strings, and contains no runtime or terminal implementation.
package reconnect

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type Policy struct {
	Attempts int
	Base     time.Duration
	Maximum  time.Duration
}

// Backoff is an unbounded retry schedule whose delay has a finite ceiling.
// The operation owner, rather than an attempt budget, decides its lifetime.
type Backoff struct {
	Base    time.Duration
	Maximum time.Duration
}

func (backoff Backoff) Delay(failure int) time.Duration {
	base := max(backoff.Base, 0)
	maximum := max(backoff.Maximum, base)
	delay := base
	for range max(failure-1, 0) {
		if delay >= maximum/2 {
			return maximum
		}
		delay *= 2
	}
	return min(delay, maximum)
}

func New(attempts int) Policy {
	return Policy{Attempts: max(attempts, 0), Base: 50 * time.Millisecond, Maximum: time.Second}
}

// Next reports the delay before retrying failure number n, counted from one.
func (r Policy) Next(n int, err error) (time.Duration, bool) {
	if n <= 0 || n > r.Attempts || !Retryable(err) {
		return 0, false
	}
	base := max(r.Base, 0)
	maximum := r.Maximum
	if maximum <= 0 {
		maximum = time.Second
	}
	delay := base
	for range n - 1 {
		if delay >= maximum/2 {
			delay = maximum
			break
		}
		delay *= 2
	}
	return min(delay, maximum), true
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

// Retryable reports whether retrying can repair the classified transport
// failure. Business, validation, and compatibility errors are permanent.
func Retryable(err error) bool {
	return errors.Is(err, agent.ErrDisconnected)
}
