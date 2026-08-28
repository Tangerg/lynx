package runs

import (
	"errors"
	"fmt"
	"iter"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/scope/app/runtime/internal/domain/approval"
	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
)

var (
	// ErrSessionBusy reports that a session or its working tree cannot admit a
	// new run segment.
	ErrSessionBusy = errors.New("runs: session busy")
	// ErrRunAdmissionBusy is the retryable subset of ErrSessionBusy: the
	// admission gate observed a current Session or working-tree owner. A caller
	// may wait on WaitSessionStartable and retry. Other busy
	// outcomes (for example a durable conflict) are not implicitly retryable.
	ErrRunAdmissionBusy = fmt.Errorf("%w: run admission busy", ErrSessionBusy)
	// ErrIsolationUnavailable reports that an isolated session cannot run because
	// isolation is not configured or the host has no sandbox backend. The run is
	// refused rather than run unconfined (fail-closed).
	ErrIsolationUnavailable = errors.New("runs: sandbox isolation unavailable")
	// ErrRunNotFound reports that a cancel or steer target is neither live nor
	// parked.
	ErrRunNotFound = errors.New("runs: run not found")
	// ErrChildRunNotAllowed reports that a caller addressed a child Run without
	// the child-run capability.
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
	// ErrExecutionClaimed and ErrExecutorNotLive are executor ownership outcomes
	// used by Resume to distinguish a concurrent claim from rehydration.
	ErrExecutionClaimed = errors.New("runs: parked execution already claimed")
	ErrExecutorNotLive  = errors.New("runs: executor not live")
	// ErrExecutorStateLost reports that a parked execution has no compatible
	// durable executor checkpoint and the application Run must be recovered lost.
	ErrExecutorStateLost = errors.New("runs: executor state lost")

	ErrInputRequired             = errors.New("runs: input required")
	ErrUnsupportedMedia          = errors.New("runs: unsupported media")
	ErrUnsupportedModelSelection = errors.New("runs: unsupported model selection")
	ErrInvalidRunLimit           = errors.New("runs: invalid run limit")
	ErrInvalidRunOptions         = errors.New("runs: invalid run options")
	// ErrInvalidCancellationReason reports a cancellation note that cannot be
	// represented by the Runtime product contract.
	ErrInvalidCancellationReason = errors.New("runs: invalid cancellation reason")
	// ErrInvalidScheduledStart reports an internal start command that carries
	// only part of the durable schedule-occurrence identity. A scheduled run
	// must be all-or-nothing: its Run, Session, and occurrence rows are one
	// opening transaction, never independently creatable facts.
	ErrInvalidScheduledStart = errors.New("runs: invalid scheduled start")
)

// InputBlockError identifies one invalid field in canonical user content while
// preserving the application error category callers branch on.
type InputBlockError struct {
	Index  int
	Field  string
	Detail string
	Cause  error
}

func (i *InputBlockError) Error() string {
	return fmt.Sprintf("runs: input block %d %s %s", i.Index, i.Field, i.Detail)
}

func (i *InputBlockError) Unwrap() error { return i.Cause }

func invalidInputBlock(index int, field, detail string, cause error) error {
	return &InputBlockError{Index: index, Field: field, Detail: detail, Cause: cause}
}

// StartCommand is the complete input for starting a Run.
type StartCommand struct {
	// RunID and NewSessionID are set only by a durable scheduled occurrence.
	// They make re-dispatch after a crash resume the same logical run/session.
	RunID                string
	NewSessionID         string
	ScheduleFiring       string
	SessionID            string
	DefaultWorkspacePath string
	NewSessionTitle      string
	ModelSelection       modelref.Selection
	Limits               run.Limits
	Options              *corechat.Options
	// Capabilities is the optional behavior enabled for this Run, already resolved
	// by the caller against what this build can execute. The use case freezes it at
	// admission rather than deriving or renegotiating it later.
	Capabilities run.Capabilities
	Input        []transcript.ContentBlock
	// GoalIncarnationID stamps a Goal-mode autonomous run with the goal incarnation
	// that launched it, so the Run's reported outcome only affects that Goal
	// (see the goals application store's incarnation-and-revision CAS). Empty for ordinary runs.
	GoalIncarnationID string
}

// ValidateScheduledIdentity ensures the three stable identifiers supplied by a
// scheduler travel as one capability. Ordinary starts leave all three empty.
// Keeping this at the command boundary prevents a future caller from creating
// a caller-chosen Session or Run without the occurrence that makes retries
// safe.
func (s StartCommand) ValidateScheduledIdentity() error {
	scheduled := s.RunID != "" || s.NewSessionID != "" || s.ScheduleFiring != ""
	if !scheduled {
		return nil
	}
	if s.RunID == "" || s.NewSessionID == "" || s.ScheduleFiring == "" {
		return fmt.Errorf("%w: run ID, new session ID, and schedule firing are required together", ErrInvalidScheduledStart)
	}
	if s.SessionID != "" {
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
			if block.MediaType != "" || len(block.Bytes) != 0 {
				return corechat.Message{}, invalidInputBlock(index, "type", "text content cannot carry media", ErrUnsupportedMedia)
			}
			parts = append(parts, corechat.NewTextPart(block.Text))
		case transcript.ImageContent:
			if block.Text != "" {
				return corechat.Message{}, invalidInputBlock(index, "type", "image content cannot carry text", ErrUnsupportedMedia)
			}
			if !strings.HasPrefix(block.MediaType, "image/") {
				return corechat.Message{}, invalidInputBlock(
					index, "mediaType", fmt.Sprintf("must be an image media type, got %q", block.MediaType), ErrUnsupportedMedia,
				)
			}
			if len(block.Bytes) == 0 {
				return corechat.Message{}, invalidInputBlock(index, "bytes", "must not be empty", ErrUnsupportedMedia)
			}
			image, mediaErr := media.NewBytes(block.MediaType, block.Bytes)
			if mediaErr != nil {
				return corechat.Message{}, invalidInputBlock(index, "bytes", mediaErr.Error(), ErrUnsupportedMedia)
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
func (s StartCommand) MaterializeInput() (message string, images []*media.Media, openingText string, err error) {
	userMessage, err := MaterializeUserMessage(s.Input)
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

// ResumeCommand is the complete input for resuming a waiting Run.
type ResumeCommand struct {
	RunID     string
	Responses []ResumeResponse
	// Input is optional user content whose transcript Item commits with the
	// continuation opening. Its model-visible Conversation projection follows the
	// answered Tool result at the first safe model boundary.
	Input []transcript.ContentBlock
	// CallerCapabilities is what this request can handle. A resume does not
	// renegotiate the Run's frozen capabilities; a caller missing any of them is
	// refused rather than served reduced behavior.
	CallerCapabilities run.Capabilities
}

// CommittedUserInput is optional follow-up content whose transcript Item has
// already committed with a continuation opening. The executor must make it
// visible after the interrupt answers at the same Strategy boundary, append it
// to Conversation once, and never project a second transcript Item.
type CommittedUserInput struct {
	ItemID  string
	Content []transcript.ContentBlock
}

type ResumeResponseKind string

const (
	ApprovalResponseKind ResumeResponseKind = "approval"
	QuestionResponseKind ResumeResponseKind = "question"
)

// ResumeResponse is the answer to one durable interrupt item.
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
// malformed.
type QuestionAnswerError struct {
	ItemID string
	Index  int
	Detail string
}

func (q *QuestionAnswerError) Error() string {
	if q.Index < 0 {
		return fmt.Sprintf("question item %q answers: %s", q.ItemID, q.Detail)
	}
	return fmt.Sprintf("question item %q answer %d: %s", q.ItemID, q.Index, q.Detail)
}

func (q *QuestionAnswerError) Unwrap() error { return ErrInvalidInterruptResponse }

// CancelCommand abandons a live or parked run.
type CancelCommand struct {
	RunID  string
	Reason string
	// AllowChildRun is the caller's already-negotiated authority to address a
	// child directly. It is false for the minimal capability set and while the
	// runtime has no child-run producer.
	AllowChildRun bool
}

const (
	defaultCancelReason = "user canceled the run"

	// MaxCancellationReasonCharacters bounds the product-facing cancellation
	// note stored in Run history. It is expressed in Unicode code points so the
	// application and every generated wire binding apply the same user-visible
	// rule independent of the caller's encoding.
	MaxCancellationReasonCharacters = 1024
)

// normalizeReason normalizes the optional product-facing note once at the
// application boundary. Every cancellation topology then records and forwards
// the same non-empty reason, independent of which caller initiated it.
func (c CancelCommand) normalizeReason() (CancelCommand, error) {
	c.Reason = strings.TrimSpace(c.Reason)
	if c.Reason == "" {
		c.Reason = defaultCancelReason
	}
	if !utf8.ValidString(c.Reason) || utf8.RuneCountInString(c.Reason) > MaxCancellationReasonCharacters {
		return CancelCommand{}, fmt.Errorf(
			"%w: reason must be valid UTF-8 containing at most %d characters",
			ErrInvalidCancellationReason,
			MaxCancellationReasonCharacters,
		)
	}
	return c, nil
}

// CancelResult is the exact durable terminal snapshot committed by Cancel.
// RootRun is present only when the addressed Run is a child, so callers do not
// have to query or reconstruct state after the command boundary.
type CancelResult struct {
	Run     run.Run
	RootRun *run.Run
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
	// CallerCapabilities is what this request can handle. It must cover the Run's
	// frozen capabilities before the subscriber attaches to the stream.
	CallerCapabilities run.Capabilities
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

// StartResult identifies the admitted Segment and exposes its event stream.
type StartResult struct {
	RunID     string
	SegmentID string
	SessionID string
	// UserItemID is empty only for an application-authored autonomous Goal
	// opening. An externally authored user start still receives the durable
	// opening userMessage identity promised by the application contract.
	UserItemID string
	Events     iter.Seq[Event]
}

// Validate checks the semantic Run-opening invariants before any Session is
// created or mutated. The Coordinator performs selected-model input admission
// separately because external capability policy belongs behind its own port.
func (r RootExecutionStart) Validate() error {
	if len(r.WorkingContext) == 0 && r.Message == "" && len(r.Media) == 0 {
		return ErrInputRequired
	}
	for index, message := range r.WorkingContext {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("runs: working context message[%d]: %w", index, err)
		}
	}
	if len(r.WorkingContext) > 0 && r.WorkingContext[len(r.WorkingContext)-1].Role != corechat.RoleUser {
		return errors.New("runs: fresh working context must end with the current user message")
	}
	if err := r.Limits.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRunLimit, err)
	}
	if err := r.ModelSelection.Validate(); err != nil {
		return fmt.Errorf("runs: model selection: %w", err)
	}
	if err := (run.Capabilities{
		ChildRuns:      r.ChildRunAdmissionEnabled,
		InterruptKinds: r.InterruptKinds,
	}).Validate(); err != nil {
		return fmt.Errorf("runs: capabilities: %w", err)
	}
	if r.GoalIncarnationID != strings.TrimSpace(r.GoalIncarnationID) {
		return errors.New("runs: goal incarnation ID has surrounding whitespace")
	}
	return validateOptions(r.Options)
}

func validateOptions(options *corechat.Options) error {
	if options == nil {
		return nil
	}
	if err := options.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRunOptions, err)
	}
	if options.Model != "" {
		return fmt.Errorf("%w: Options.Model must stay empty; use Provider and Model", ErrInvalidRunOptions)
	}
	if options.ReasoningEffort != "" {
		return fmt.Errorf("%w: Options.ReasoningEffort must stay empty; use ModelSelection", ErrInvalidRunOptions)
	}
	return nil
}

// ActiveRunConflictError reports that a Session already holds a non-terminal root Run,
// so a new one cannot be admitted. It carries the Run because the caller's next move
// depends on which one it is and what it is doing — steer it, answer it, or cancel it
// — and because the runtime will not choose for them: an implicit cancel would throw
// away work to serve a request that might have been meant as a steer.
//
// It is a typed error rather than a sentinel: the identity IS the information, and a
// sentinel would leave the caller to go find out what blocked it.
type ActiveRunConflictError struct {
	RunID  string
	Status run.Status
}

func (a *ActiveRunConflictError) Error() string {
	return fmt.Sprintf("runs: session already has a %s run %q", a.Status, a.RunID)
}
