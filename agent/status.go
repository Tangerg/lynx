package agent

import (
	"errors"
	"fmt"
)

// ErrInvalidStatus reports an unknown common Process lifecycle state.
var ErrInvalidStatus = errors.New("agent: invalid status")

// Status is the complete common lifecycle state of a Process. Strategy-specific
// conditions such as a Planning no-plan result do not add common statuses.
type Status uint8

const (
	// StatusInvalid is the invalid zero value.
	StatusInvalid Status = iota
	// StatusNotStarted identifies a Process before execution begins.
	StatusNotStarted
	// StatusRunning identifies a Process eligible to advance.
	StatusRunning
	// StatusWaiting identifies a Process awaiting a WaitID-addressed Signal.
	StatusWaiting
	// StatusPaused identifies an explicitly suspended Process.
	StatusPaused
	// StatusCompleted identifies successful semantic completion.
	StatusCompleted
	// StatusFailed identifies terminal execution failure.
	StatusFailed
	// StatusCanceled identifies cooperative cancellation.
	StatusCanceled
	// StatusTimedOut identifies deadline termination.
	StatusTimedOut
	// StatusKilled identifies an explicit Engine kill.
	StatusKilled
)

// String returns the stable lifecycle-state name.
func (status Status) String() string {
	switch status {
	case StatusNotStarted:
		return "not_started"
	case StatusRunning:
		return "running"
	case StatusWaiting:
		return "waiting"
	case StatusPaused:
		return "paused"
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	case StatusCanceled:
		return "canceled"
	case StatusTimedOut:
		return "timed_out"
	case StatusKilled:
		return "killed"
	default:
		return "invalid"
	}
}

func parseStatus(value string) (Status, error) {
	switch value {
	case "not_started":
		return StatusNotStarted, nil
	case "running":
		return StatusRunning, nil
	case "waiting":
		return StatusWaiting, nil
	case "paused":
		return StatusPaused, nil
	case "completed":
		return StatusCompleted, nil
	case "failed":
		return StatusFailed, nil
	case "canceled":
		return StatusCanceled, nil
	case "timed_out":
		return StatusTimedOut, nil
	case "killed":
		return StatusKilled, nil
	default:
		return StatusInvalid, fmt.Errorf("%w: unknown value %q", ErrInvalidStatus, value)
	}
}

// Valid reports whether status is a defined lifecycle value.
func (status Status) Valid() bool { return status >= StatusNotStarted && status <= StatusKilled }

// Terminal reports whether the Process may never transition again.
func (status Status) Terminal() bool { return status >= StatusCompleted && status <= StatusKilled }

func (status Status) canTransitionTo(next Status) bool {
	switch status {
	case StatusNotStarted:
		return next == StatusRunning
	case StatusRunning:
		return next == StatusWaiting || next == StatusPaused || next.Terminal()
	case StatusWaiting:
		return next == StatusRunning || next == StatusFailed || next == StatusCanceled || next == StatusTimedOut || next == StatusKilled
	case StatusPaused:
		return next == StatusRunning || next == StatusCanceled || next == StatusTimedOut || next == StatusKilled
	default:
		return false
	}
}

// MarshalText returns the validated stable lifecycle-state name.
func (status Status) MarshalText() ([]byte, error) {
	if !status.Valid() {
		return nil, ErrInvalidStatus
	}
	return []byte(status.String()), nil
}

// UnmarshalText replaces status with a parsed lifecycle state.
func (status *Status) UnmarshalText(text []byte) error {
	if status == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidStatus)
	}
	value, err := parseStatus(string(text))
	if err != nil {
		return err
	}
	*status = value
	return nil
}
