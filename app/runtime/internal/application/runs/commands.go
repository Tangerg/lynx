package runs

import (
	"encoding/base64"
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/pkg/mime"
)

var (
	// ErrSessionBusy reports that a session or its working tree cannot admit a
	// new run segment.
	ErrSessionBusy = errors.New("runs: session busy")
	// ErrIsolationUnavailable reports that an isolated session cannot run because
	// isolation is not configured or the host has no sandbox backend. The run is
	// refused rather than run unconfined (fail-closed).
	ErrIsolationUnavailable = errors.New("runs: sandbox isolation unavailable")
	// ErrRunNotFound reports that a cancel or steer target is neither live nor
	// parked.
	ErrRunNotFound = errors.New("runs: run not found")
	// ErrInterruptNotOpen reports that a resume target has no open interrupt.
	ErrInterruptNotOpen = errors.New("runs: interrupt not open")
	// ErrInvalidInterruptResponse reports a response set that does not exactly
	// cover the open interrupt schema.
	ErrInvalidInterruptResponse = errors.New("runs: invalid interrupt response")
	// ErrParkClaimed and ErrTurnNotLive are executor ownership outcomes used by
	// Resume to distinguish a concurrent claim from a process rehydrate.
	ErrParkClaimed = errors.New("runs: parked turn already claimed")
	ErrTurnNotLive = errors.New("runs: turn not live")
	// ErrTurnStateLost reports that a parked executor turn has no compatible
	// durable process state and the application Run must be recovered lost.
	ErrTurnStateLost = errors.New("runs: turn state lost")

	ErrInputRequired      = errors.New("runs: input required")
	ErrUnsupportedMedia   = errors.New("runs: unsupported media")
	ErrInvalidTurnLimit   = errors.New("runs: invalid turn limit")
	ErrInvalidTurnOptions = errors.New("runs: invalid turn options")
	// ErrInvalidScheduledStart reports an internal start command that carries
	// only part of the durable schedule-occurrence identity. A scheduled run
	// must be all-or-nothing: its Run, Session, and occurrence rows are one
	// opening transaction, never independently creatable facts.
	ErrInvalidScheduledStart = errors.New("runs: invalid scheduled start")
)

// StartCommand is the protocol-neutral runs.start use case input.
type StartCommand struct {
	// RunID and NewSessionID are set only by a durable scheduled occurrence.
	// They make re-dispatch after a crash resume the same logical run/session.
	RunID           string
	NewSessionID    string
	ScheduleFiring  string
	SessionID       string
	DefaultCwd      string
	NewSessionTitle string
	ModelSelection  modelref.Selection
	MaxBudget       int64
	MaxCostUSD      float64
	MaxSteps        int
	Options         *corechat.Options
	// ProtocolProfile is the protocol contract negotiated for this Run, already
	// resolved against what this build advertises. It is an input rather than
	// something the use case derives: what a client declared is a wire fact, and
	// the use case's job is to freeze it onto the Run.
	ProtocolProfile execution.RunProtocolProfile
	Input           []transcript.ContentBlock
	// GoalLeaseID stamps a Goal-mode autonomous run with the goal incarnation
	// that launched it, so the run's update_goal signal only affects that goal
	// (see the goals application store's lease-and-revision CAS). Empty for ordinary runs.
	GoalLeaseID string
}

// ValidateScheduledIdentity ensures the three stable identifiers supplied by a
// scheduler travel as one capability. Ordinary starts leave all three empty.
// Keeping this at the command boundary prevents a future caller from creating
// a caller-chosen Session or Run without the occurrence that makes retries
// safe.
func (c StartCommand) ValidateScheduledIdentity() error {
	scheduled := c.RunID != "" || c.NewSessionID != "" || c.ScheduleFiring != ""
	if !scheduled {
		return nil
	}
	if c.RunID == "" || c.NewSessionID == "" || c.ScheduleFiring == "" {
		return fmt.Errorf("%w: run ID, new session ID, and schedule firing are required together", ErrInvalidScheduledStart)
	}
	if c.SessionID != "" {
		return fmt.Errorf("%w: scheduled start cannot also select an existing session", ErrInvalidScheduledStart)
	}
	return nil
}

// MaterializeInput derives the executor message/media pair and the durable
// opening-item text from the one canonical input representation. Keeping this
// conversion in Application prevents adapters from supplying three potentially
// divergent descriptions of a user turn.
func (c StartCommand) MaterializeInput() (message string, images []*media.Media, openingText string, err error) {
	texts := make([]string, 0, len(c.Input))
	for index, block := range c.Input {
		switch block.Kind {
		case transcript.TextContent:
			if block.Text != "" {
				texts = append(texts, block.Text)
			}
		case transcript.ImageContent:
			parsed, parseErr := mime.Parse(block.Mime)
			if parseErr != nil || !mime.IsImage(parsed) {
				return "", nil, "", fmt.Errorf("%w: input block %d has unsupported image mime %q", ErrUnsupportedMedia, index, block.Mime)
			}
			if block.Data == "" {
				return "", nil, "", fmt.Errorf("%w: input block %d has empty image data", ErrUnsupportedMedia, index)
			}
			data, decodeErr := base64.StdEncoding.DecodeString(block.Data)
			if decodeErr != nil {
				return "", nil, "", fmt.Errorf("%w: input block %d image data is not valid base64: %v", ErrUnsupportedMedia, index, decodeErr)
			}
			image, mediaErr := media.NewBytes(parsed.TypeAndSubType(), data)
			if mediaErr != nil {
				return "", nil, "", fmt.Errorf("%w: input block %d: %v", ErrUnsupportedMedia, index, mediaErr)
			}
			images = append(images, image)
		default:
			return "", nil, "", fmt.Errorf("%w: input block %d has unknown content kind", ErrUnsupportedMedia, index)
		}
	}
	message = strings.Join(texts, "\n")
	return message, images, strings.TrimSpace(message), nil
}

// ResumeCommand is the protocol-neutral runs.resume use case input.
type ResumeCommand struct {
	RunID     string
	Responses []ResumeResponse
	// CallerCapabilities is what THIS request declares it can handle. A resume does
	// not renegotiate the Run's frozen profile — it is only checked against it, so a
	// caller that could not follow the Run's stream is refused rather than served a
	// quietly reduced one.
	CallerCapabilities execution.RunProtocolProfile
}

type ResumeResponseKind string

const (
	ApprovalResponseKind ResumeResponseKind = "approval"
	QuestionResponseKind ResumeResponseKind = "question"
)

// ResumeResponse is the protocol-neutral answer to one durable interrupt item.
// Exactly one payload must match Kind.
type ResumeResponse struct {
	ItemID   string
	Kind     ResumeResponseKind
	Approval *ApprovalResponse
	Question *QuestionResponse
}

type ApprovalResponse struct {
	Approved      bool
	Arguments     string
	Reason        string
	RememberScope approval.Scope
}

type QuestionResponse struct {
	Answers map[string][]string
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
	RunID      string
	SegmentID  string
	SessionID  string
	UserItemID string
	Events     iter.Seq[Event]
}

// Validate checks the transport-neutral turn invariants before any session is
// created or mutated. Adapter-specific model modality checks are performed by
// TurnControl.ValidateStart in the same pre-admission phase.
func (r StartTurn) Validate() error {
	if r.Message == "" && len(r.Media) == 0 {
		return ErrInputRequired
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
	if err := options.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidTurnOptions, err)
	}
	if options.Model != "" {
		return fmt.Errorf("%w: Options.Model must stay empty; use Provider and Model", ErrInvalidTurnOptions)
	}
	return nil
}

// ActiveRunConflict reports that a Session already holds a non-terminal root Run,
// so a new one cannot be admitted. It carries the Run because the caller's next move
// depends on which one it is and what it is doing — steer it, answer it, or cancel it
// — and because the runtime will not choose for them: an implicit cancel would throw
// away work to serve a request that might have been meant as a steer.
//
// It is a typed error rather than a sentinel: the identity IS the information, and a
// sentinel would leave the caller to go find out what blocked it.
type ActiveRunConflict struct {
	RunID  string
	Status execution.RunStatus
}

func (e *ActiveRunConflict) Error() string {
	return fmt.Sprintf("runs: session already has a %s run %q", e.Status, e.RunID)
}
