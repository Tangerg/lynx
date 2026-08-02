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
	// ErrChildRunNotAllowed reports that a caller addressed a child Run without
	// the child-run capability. The application keeps this protocol-neutral:
	// Delivery decides how that missing authority is represented on its wire.
	ErrChildRunNotAllowed = errors.New("runs: child run control is not allowed")
	// ErrRunWaiting and ErrRunFinished report a run that exists but is not
	// executing. They are separate from ErrRunNotFound because the caller's next
	// move differs and is knowable: a waiting run needs its interrupts answered, a
	// finished one has nothing left but its transcript.
	ErrRunWaiting  = errors.New("runs: run is waiting for a response")
	ErrRunFinished = errors.New("runs: run has finished")
	// ErrStaleSegment reports that the run is executing a segment other than the
	// one the caller addressed. The caller is acting on an execution that has been
	// replaced — steering it would inject into work the user never saw, and
	// subscribing to it would fold a stream the client believes is a different one.
	ErrStaleSegment = errors.New("runs: run is executing a different segment")
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
	// durable executor checkpoint and the application Run must be recovered lost.
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

// InputBlockError identifies one invalid field in canonical user content while
// preserving the application error category callers branch on. Delivery may
// translate Index and Field into its wire path without re-validating media.
type InputBlockError struct {
	Index  int
	Field  string
	Detail string
	Cause  error
}

func (e *InputBlockError) Error() string {
	return fmt.Sprintf("runs: input block %d %s %s", e.Index, e.Field, e.Detail)
}

func (e *InputBlockError) Unwrap() error { return e.Cause }

func invalidInputBlock(index int, field, detail string, cause error) error {
	return &InputBlockError{Index: index, Field: field, Detail: detail, Cause: cause}
}

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
	Limits          execution.RunLimits
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

// MaterializeUserMessage converts canonical content blocks into the
// provider-neutral chat message consumed by the executor. Start and steer share
// this one conversion so image validation and block ordering cannot drift.
func MaterializeUserMessage(input []transcript.ContentBlock) (corechat.Message, error) {
	parts := make([]corechat.Part, 0, len(input))
	for index, block := range input {
		switch block.Kind {
		case transcript.TextContent:
			if block.Text == "" {
				return corechat.Message{}, invalidInputBlock(index, "text", "must not be empty", ErrInputRequired)
			}
			if block.Mime != "" || block.Data != "" {
				return corechat.Message{}, invalidInputBlock(index, "type", "text content cannot carry mime or data", ErrUnsupportedMedia)
			}
			parts = append(parts, corechat.NewTextPart(block.Text))
		case transcript.ImageContent:
			if block.Text != "" {
				return corechat.Message{}, invalidInputBlock(index, "type", "image content cannot carry text", ErrUnsupportedMedia)
			}
			parsed, parseErr := mime.Parse(block.Mime)
			if parseErr != nil || !mime.IsImage(parsed) {
				return corechat.Message{}, invalidInputBlock(
					index, "mime", fmt.Sprintf("must be a supported image MIME, got %q", block.Mime), ErrUnsupportedMedia,
				)
			}
			if block.Data == "" {
				return corechat.Message{}, invalidInputBlock(index, "data", "must not be empty", ErrUnsupportedMedia)
			}
			data, decodeErr := base64.StdEncoding.DecodeString(block.Data)
			if decodeErr != nil {
				return corechat.Message{}, invalidInputBlock(index, "data", "must be valid base64", ErrUnsupportedMedia)
			}
			image, mediaErr := media.NewBytes(parsed.TypeAndSubType(), data)
			if mediaErr != nil {
				return corechat.Message{}, invalidInputBlock(index, "data", mediaErr.Error(), ErrUnsupportedMedia)
			}
			parts = append(parts, corechat.NewMediaPart(image))
		default:
			return corechat.Message{}, invalidInputBlock(index, "type", "is unknown", ErrUnsupportedMedia)
		}
	}
	if len(parts) == 0 {
		return corechat.Message{}, ErrInputRequired
	}
	message := corechat.NewUserMessage(parts...)
	if err := message.Validate(); err != nil {
		return corechat.Message{}, fmt.Errorf("runs: materialized user message: %w", err)
	}
	return message, nil
}

// MaterializeInput projects the validated user message onto the executor's
// opening prompt plus image-attachment boundary and derives the durable opening
// text. Steering consumes the ordered message directly.
func (c StartCommand) MaterializeInput() (message string, images []*media.Media, openingText string, err error) {
	userMessage, err := MaterializeUserMessage(c.Input)
	if err != nil {
		return "", nil, "", err
	}
	texts := make([]string, 0, len(userMessage.Parts))
	for _, part := range userMessage.Parts {
		switch part.Kind {
		case corechat.PartText:
			texts = append(texts, part.Text)
		case corechat.PartMedia:
			images = append(images, part.Media)
		}
	}
	message = strings.Join(texts, "\n")
	return message, images, strings.TrimSpace(message), nil
}

// ResumeCommand is the protocol-neutral runs.resume use case input.
type ResumeCommand struct {
	RunID     string
	Responses []ResumeResponse
	// Input is an optional user turn committed with the continuation. It rides the
	// same opening write-set as the resume itself, so "answered the interrupt" and
	// "said this as well" cannot come apart.
	Input []transcript.ContentBlock
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
	Answers [][]string
}

// QuestionAnswerError identifies the exact ordered answer that failed the
// durable question schema. Index is -1 when the answer collection itself is
// malformed. Delivery turns it into invalid_params.errors without teaching the
// application about JSON field paths.
type QuestionAnswerError struct {
	ItemID string
	Index  int
	Detail string
}

func (e *QuestionAnswerError) Error() string {
	if e.Index < 0 {
		return fmt.Sprintf("question item %q answers: %s", e.ItemID, e.Detail)
	}
	return fmt.Sprintf("question item %q answer %d: %s", e.ItemID, e.Index, e.Detail)
}

func (e *QuestionAnswerError) Unwrap() error { return ErrInvalidInterruptResponse }

// CancelCommand abandons a live or parked run.
type CancelCommand struct {
	RunID  string
	Reason string
	// AllowChildRun is the caller's already-negotiated authority to address a
	// child directly. It is false for the Minimal Profile and while the runtime
	// has no child-run producer.
	AllowChildRun bool
}

// CancelResult is the exact durable terminal snapshot committed by Cancel.
// RootRun is present only when the addressed Run is a child; it gives Delivery
// enough information to publish the closed root/child result union without
// querying or reconstructing domain state after the command boundary.
type CancelResult struct {
	Run     transcript.Run
	RootRun *transcript.Run
}

// SteerCommand injects structured user content into an actively executing run.
type SteerCommand struct {
	RunID string
	// ExpectedSegmentID is the segment the caller believes is executing. It is
	// required: without it "inject into whatever is running now" would deliver the
	// user's instruction to a continuation they never saw — the run they meant
	// could have parked and been resumed between typing and sending.
	ExpectedSegmentID string
	Input             []transcript.ContentBlock
}

// SubscribeRequest attaches a caller to a run's live segment.
type SubscribeRequest struct {
	RunID string
	// SegmentID is the segment the caller believes is executing, required for the
	// same reason a steer needs it: a reconnect that says only "this run" would
	// attach to a stream that is not the one it was folding.
	SegmentID string
	// Cursor is the opaque position the caller last folded, empty for a fresh
	// attach. Empty is tail-only — history comes from the transcript reads.
	Cursor string
	// Caller is what THIS request declares it can handle, checked against the
	// Run's frozen profile. A subscriber that could not follow the stream is
	// refused rather than served a quietly reduced one.
	Caller execution.RunProtocolProfile
}

// Subscription is an attached caller's view of a live segment.
type Subscription struct {
	Record Record
	// HeadCursor is the stream's position when the subscription attached, empty
	// when nothing had been published yet. The caller stores it verbatim and hands
	// it back on the next reconnect; it is not orderable and must not be parsed.
	HeadCursor string
	Events     iter.Seq[Event]
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
	if err := r.Limits.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidTurnLimit, err)
	}
	if err := r.ModelSelection.Validate(); err != nil {
		return fmt.Errorf("runs: turn model selection: %w", err)
	}
	if err := (execution.RunProtocolProfile{
		ChildRuns:      r.ChildRunAdmissionEnabled,
		InterruptKinds: r.InterruptKinds,
	}).Validate(); err != nil {
		return fmt.Errorf("runs: turn protocol profile: %w", err)
	}
	if r.GoalLeaseID != strings.TrimSpace(r.GoalLeaseID) {
		return errors.New("runs: turn goal lease ID has surrounding whitespace")
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
