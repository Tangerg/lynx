package agent2

import (
	"errors"
	"fmt"
)

var ErrInvalidStatus = errors.New("agent: invalid status")

// Status is the complete common lifecycle state of a Process. Strategy-specific
// conditions such as a Planning no-plan result do not add common statuses.
type Status uint8

const (
	StatusInvalid Status = iota
	StatusNotStarted
	StatusRunning
	StatusWaiting
	StatusPaused
	StatusCompleted
	StatusFailed
	StatusCancelled
	StatusTimedOut
	StatusKilled
)

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
	case StatusCancelled:
		return "cancelled"
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
	case "cancelled":
		return StatusCancelled, nil
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

// CanTransitionTo reports whether the common Process state machine permits one
// committed transition from status to next. Terminal statuses always return
// false, which enforces first-terminal-wins.
func (status Status) CanTransitionTo(next Status) bool {
	switch status {
	case StatusNotStarted:
		return next == StatusRunning
	case StatusRunning:
		return next == StatusWaiting || next == StatusPaused || next.Terminal()
	case StatusWaiting:
		return next == StatusRunning || next == StatusFailed || next == StatusCancelled || next == StatusTimedOut || next == StatusKilled
	case StatusPaused:
		return next == StatusRunning || next == StatusCancelled || next == StatusTimedOut || next == StatusKilled
	default:
		return false
	}
}

func (status Status) MarshalText() ([]byte, error) {
	if !status.Valid() {
		return nil, ErrInvalidStatus
	}
	return []byte(status.String()), nil
}

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
