// Package resilience owns transport retry policy shared by interactive and
// headless delivery adapters. It classifies symbolic client errors, never error
// strings, and contains no runtime or terminal implementation.
package resilience

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

type Reconnect struct {
	Attempts int
	Base     time.Duration
	Maximum  time.Duration
}

func Standard(attempts int) Reconnect {
	return Reconnect{Attempts: max(attempts, 0), Base: 50 * time.Millisecond, Maximum: time.Second}
}

// Next reports the delay before retrying failure number n, counted from one.
func (r Reconnect) Next(n int, err error) (time.Duration, bool) {
	if n <= 0 || n > r.Attempts || !retryable(err) {
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

func retryable(err error) bool {
	return errors.Is(err, client.ErrDisconnected) || errors.Is(err, client.ErrEventGap)
}
