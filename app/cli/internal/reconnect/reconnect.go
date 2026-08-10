// Package reconnect owns transport retry policy shared by interactive and
// headless delivery adapters. It classifies symbolic client errors, never error
// strings, and contains no runtime or terminal implementation.
package reconnect

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

type Policy struct {
	Attempts int
	Base     time.Duration
	Maximum  time.Duration
}

func New(attempts int) Policy {
	return Policy{Attempts: max(attempts, 0), Base: 50 * time.Millisecond, Maximum: time.Second}
}

// Next reports the delay before retrying failure number n, counted from one.
func (r Policy) Next(n int, err error) (time.Duration, bool) {
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

// Control retries an idempotent control operation after ambiguous transport
// failures using the same bounded policy as stream reconnects.
func Control(ctx context.Context, policy Policy, operation func() error) error {
	_, err := ControlValue(ctx, policy, func() (struct{}, error) {
		return struct{}{}, operation()
	})
	return err
}

// ControlValue is Control for an idempotent operation that returns a value.
func ControlValue[T any](ctx context.Context, policy Policy, operation func() (T, error)) (T, error) {
	var zero T
	for failure := 1; ; failure++ {
		value, err := operation()
		if err == nil {
			return value, nil
		}
		if !errors.Is(err, client.ErrDisconnected) {
			return zero, err
		}
		delay, retry := policy.Next(failure, err)
		if !retry {
			return zero, err
		}
		if err := Wait(ctx, delay); err != nil {
			return zero, err
		}
	}
}

func retryable(err error) bool {
	return errors.Is(err, client.ErrDisconnected) || errors.Is(err, client.ErrEventGap)
}
