package runs

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/media"
	corechat "github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

var (
	// ErrSessionBusy reports that a session or its working tree cannot admit a
	// new run segment.
	ErrSessionBusy = errors.New("runs: session busy")
	// ErrRunNotFound reports that a cancel or steer target is neither live nor
	// parked.
	ErrRunNotFound = errors.New("runs: run not found")
	// ErrInterruptNotOpen reports that a resume target has no open interrupt.
	ErrInterruptNotOpen = errors.New("runs: interrupt not open")
	// ErrParkClaimed and ErrTurnNotLive are executor ownership outcomes used by
	// Resume to distinguish a concurrent claim from a process rehydrate.
	ErrParkClaimed = errors.New("runs: parked turn already claimed")
	ErrTurnNotLive = errors.New("runs: turn not live")

	ErrInputRequired            = errors.New("runs: input required")
	ErrIncompleteModelSelection = errors.New("runs: incomplete model selection")
	ErrUnsupportedMedia         = errors.New("runs: unsupported media")
	ErrInvalidTurnLimit         = errors.New("runs: invalid turn limit")
	ErrInvalidTurnOptions       = errors.New("runs: invalid turn options")
)

// StartCommand is the protocol-neutral runs.start use case input. NewProjector
// is the temporary Batch-1 projection seam; all lifecycle and executor inputs
// are canonical application values.
type StartCommand struct {
	SessionID       string
	DefaultCwd      string
	NewSessionTitle string
	Message         string
	Media           []*media.Media
	Provider        string
	Model           string
	MaxBudget       int64
	MaxCostUSD      float64
	MaxSteps        int
	Options         *corechat.Options
	InterruptKinds  []string
	OpeningUserText string
	NewProjector    ProjectorFactory
}

// ResumeCommand is the protocol-neutral runs.resume use case input.
type ResumeCommand struct {
	RunID          string
	Resolution     interrupts.Resolution
	InterruptKinds []string
	NewProjector   ProjectorFactory
}

// CancelCommand abandons a live or parked run.
type CancelCommand struct {
	RunID  string
	Reason string
}

// SteerCommand injects a message into an actively executing run.
type SteerCommand struct {
	RunID   string
	Message string
}

// StartResult identifies the admitted segment and exposes its application
// event stream. Delivery only maps this result to protocol DTOs.
type StartResult struct {
	RunID     string
	SegmentID string
	SessionID string
	Events    <-chan Event
}

// Validate checks the transport-neutral turn invariants before any session is
// created or mutated. Adapter-specific model modality checks are performed by
// TurnControl.ValidateStart in the same pre-admission phase.
func (r StartTurn) Validate() error {
	if r.Message == "" && len(r.Media) == 0 {
		return ErrInputRequired
	}
	if (r.Model == "") != (r.Provider == "") {
		return ErrIncompleteModelSelection
	}
	if r.MaxBudget < 0 {
		return fmt.Errorf("%w: MaxBudget must be non-negative", ErrInvalidTurnLimit)
	}
	if r.MaxCostUSD < 0 {
		return fmt.Errorf("%w: MaxCostUSD must be non-negative", ErrInvalidTurnLimit)
	}
	if r.MaxSteps < 0 {
		return fmt.Errorf("%w: MaxSteps must be non-negative", ErrInvalidTurnLimit)
	}
	return validateOptions(r.Options)
}

func validateOptions(options *corechat.Options) error {
	if options == nil {
		return nil
	}
	if options.Model != "" {
		return fmt.Errorf("%w: Options.Model must stay empty; use Provider and Model", ErrInvalidTurnOptions)
	}
	if options.MaxTokens != nil && *options.MaxTokens <= 0 {
		return fmt.Errorf("%w: MaxTokens must be positive", ErrInvalidTurnOptions)
	}
	if options.Temperature != nil && (*options.Temperature < 0 || *options.Temperature > 2) {
		return fmt.Errorf("%w: Temperature must be between 0 and 2", ErrInvalidTurnOptions)
	}
	if options.TopP != nil && (*options.TopP < 0 || *options.TopP > 1) {
		return fmt.Errorf("%w: TopP must be between 0 and 1", ErrInvalidTurnOptions)
	}
	for _, stop := range options.Stop {
		if stop == "" {
			return fmt.Errorf("%w: Stop must not contain empty strings", ErrInvalidTurnOptions)
		}
	}
	return nil
}
