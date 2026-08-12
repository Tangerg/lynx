// Package reconnect owns transport retry policy shared by interactive and
// headless delivery adapters. It classifies symbolic agent-port errors, never error
// strings, and contains no runtime or terminal implementation.
package reconnect

import (
	"errors"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
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
	if errors.Is(err, agent.ErrCommandInProgress) {
		delay = max(delay, time.Second)
	}
	return min(delay, maximum), true
}

// Retryable reports whether retrying can repair the classified transport
// failure. Business, validation, and compatibility errors are permanent.
func Retryable(err error) bool {
	return errors.Is(err, agent.ErrDisconnected) || errors.Is(err, agent.ErrCommandInProgress)
}
