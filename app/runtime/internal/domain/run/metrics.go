package run

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
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
func (metrics Metrics) Validate() error {
	if metrics.steps < 0 || metrics.activeDuration < 0 {
		return errors.New("run: metrics must not be negative")
	}
	if metrics.usage != nil {
		if err := metrics.usage.Validate(); err != nil {
			return fmt.Errorf("run: metrics: %w", err)
		}
	}
	return nil
}

// ValidateAdvanceFrom proves that metrics is a monotonic continuation of the
// previous committed value.
func (metrics Metrics) ValidateAdvanceFrom(previous Metrics) error {
	if err := previous.Validate(); err != nil {
		return fmt.Errorf("run: previous metrics: %w", err)
	}
	if err := metrics.Validate(); err != nil {
		return err
	}
	if metrics.steps < previous.steps || metrics.activeDuration < previous.activeDuration {
		return errors.New("run: cumulative metrics regressed")
	}
	switch {
	case previous.usage != nil && metrics.usage == nil:
		return errors.New("run: cumulative usage disappeared")
	case previous.usage != nil:
		if err := metrics.usage.ValidateAdvanceFrom(*previous.usage); err != nil {
			return fmt.Errorf("run: usage: %w", err)
		}
	}
	return nil
}

// Equal reports semantic equality.
func (metrics Metrics) Equal(other Metrics) bool {
	if metrics.steps != other.steps || metrics.activeDuration != other.activeDuration {
		return false
	}
	if metrics.usage == nil || other.usage == nil {
		return metrics.usage == nil && other.usage == nil
	}
	return metrics.usage.Equal(*other.usage)
}

// Usage returns an ownership-isolated cumulative usage value.
func (metrics Metrics) Usage() (accounting.Usage, bool) {
	if metrics.usage == nil {
		return accounting.Usage{}, false
	}
	return metrics.usage.Clone(), true
}

// Steps returns the cumulative model-call count.
func (metrics Metrics) Steps() int { return metrics.steps }

// ActiveDuration returns the cumulative time spent executing.
func (metrics Metrics) ActiveDuration() time.Duration { return metrics.activeDuration }

// AddActiveDuration returns metrics with an additional completed Segment
// duration. It rejects negative durations and overflow.
func (metrics Metrics) AddActiveDuration(duration time.Duration) (Metrics, error) {
	if duration < 0 {
		return Metrics{}, errors.New("run: active duration increment must not be negative")
	}
	if duration > 0 && metrics.activeDuration > time.Duration(math.MaxInt64)-duration {
		return Metrics{}, errors.New("run: active duration overflows")
	}
	next := metrics
	next.activeDuration += duration
	return next, nil
}
