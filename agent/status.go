package agent

import (
	"errors"
	"fmt"
)

// ErrInvalidStatus reports an unknown common Process lifecycle state.
var ErrInvalidStatus = errors.New("agent: invalid status")

// Status is the complete common lifecycle state of a Process. Strategy-specific
// conditions such as a Planning no-plan result do not add common statuses.
type Status string

const (
	// StatusInvalid is the invalid zero value.
	StatusInvalid Status = ""
	// StatusNotStarted identifies a Process before execution begins.
	StatusNotStarted Status = "not_started"
	// StatusRunning identifies a Process eligible to advance.
	StatusRunning Status = "running"
	// StatusWaiting identifies a Process awaiting a WaitID-addressed Signal.
	StatusWaiting Status = "waiting"
	// StatusPaused identifies an explicitly suspended Process.
	StatusPaused Status = "paused"
	// StatusCompleted identifies successful semantic completion.
	StatusCompleted Status = "completed"
	// StatusFailed identifies terminal execution failure.
	StatusFailed Status = "failed"
	// StatusCanceled identifies cooperative cancellation.
	StatusCanceled Status = "canceled"
	// StatusTimedOut identifies deadline termination.
	StatusTimedOut Status = "timed_out"
	// StatusKilled identifies an explicit Engine kill.
	StatusKilled Status = "killed"
)

// String returns the stable lifecycle-state name.
func (status Status) String() string {
	if !status.Valid() {
		return "invalid"
	}
	return string(status)
}

func parseStatus(value string) (Status, error) {
	status := Status(value)
	if !status.Valid() {
		return StatusInvalid, fmt.Errorf("%w: unknown value %q", ErrInvalidStatus, value)
	}
	return status, nil
}

// Valid reports whether status is a defined lifecycle value.
func (status Status) Valid() bool {
	switch status {
	case StatusNotStarted, StatusRunning, StatusWaiting, StatusPaused,
		StatusCompleted, StatusFailed, StatusCanceled, StatusTimedOut, StatusKilled:
		return true
	default:
		return false
	}
}

// Terminal reports whether the Process may never transition again.
func (status Status) Terminal() bool {
	switch status {
	case StatusCompleted, StatusFailed, StatusCanceled, StatusTimedOut, StatusKilled:
		return true
	default:
		return false
	}
}

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
	return []byte(status), nil
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
