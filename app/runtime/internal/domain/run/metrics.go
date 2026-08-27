package run

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/accounting"
)

// Metrics is the cumulative consumption of one Run across all of its Segments.
// Usage is absent until a provider reports it. ActiveDuration excludes time
// spent waiting for a person.
type Metrics struct {
	usage          *accounting.Usage
	steps          int
	activeDuration time.Duration
}

// NewMetrics constructs a validated cumulative metrics value.
func NewMetrics(usage *accounting.Usage, steps int, activeDuration time.Duration) (Metrics, error) {
	metrics := Metrics{steps: steps, activeDuration: activeDuration}
	if usage != nil {
		cloned := usage.Clone()
		metrics.usage = &cloned
	}
	if err := metrics.Validate(); err != nil {
		return Metrics{}, err
	}
	return metrics, nil
}

// Validate reports whether the cumulative values are internally consistent.
func (m Metrics) Validate() error {
	if m.steps < 0 || m.activeDuration < 0 {
		return errors.New("run: metrics must not be negative")
	}
	if m.usage != nil {
		if err := m.usage.Validate(); err != nil {
			return fmt.Errorf("run: metrics: %w", err)
		}
	}
	return nil
}

// ValidateAdvanceFrom proves that metrics is a monotonic continuation of the
// previous committed value.
func (m Metrics) ValidateAdvanceFrom(previous Metrics) error {
	if err := previous.Validate(); err != nil {
		return fmt.Errorf("run: previous metrics: %w", err)
	}
	if err := m.Validate(); err != nil {
		return err
	}
	if m.steps < previous.steps || m.activeDuration < previous.activeDuration {
		return errors.New("run: cumulative metrics regressed")
	}
	switch {
	case previous.usage != nil && m.usage == nil:
		return errors.New("run: cumulative usage disappeared")
	case previous.usage != nil:
		if err := m.usage.ValidateAdvanceFrom(*previous.usage); err != nil {
			return fmt.Errorf("run: usage: %w", err)
		}
	}
	return nil
}

// Equal reports semantic equality.
func (m Metrics) Equal(other Metrics) bool {
	if m.steps != other.steps || m.activeDuration != other.activeDuration {
		return false
	}
	if m.usage == nil || other.usage == nil {
		return m.usage == nil && other.usage == nil
	}
	return m.usage.Equal(*other.usage)
}

// Usage returns an ownership-isolated cumulative usage value.
func (m Metrics) Usage() (accounting.Usage, bool) {
	if m.usage == nil {
		return accounting.Usage{}, false
	}
	return m.usage.Clone(), true
}

// Steps returns the cumulative model-call count.
func (m Metrics) Steps() int { return m.steps }

// ActiveDuration returns the cumulative time spent executing.
func (m Metrics) ActiveDuration() time.Duration { return m.activeDuration }

// AddActiveDuration returns m with an additional completed Segment
// duration. It rejects negative durations and overflow.
func (m Metrics) AddActiveDuration(duration time.Duration) (Metrics, error) {
	if duration < 0 {
		return Metrics{}, errors.New("run: active duration increment must not be negative")
	}
	if duration > 0 && m.activeDuration > time.Duration(math.MaxInt64)-duration {
		return Metrics{}, errors.New("run: active duration overflows")
	}
	next := m
	next.activeDuration += duration
	return next, nil
}
