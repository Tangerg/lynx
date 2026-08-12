// Package agent owns the CLI's agent-conversation model and runtime port.
//
// The types here are presentation models, deliberately not the runtime's wire
// contract. A terminal wants a flat, mutable, ordered transcript; the wire is
// shaped for replay, dedup and pagination. An implementation of [Runtime]
// translates one into the other — which is also what keeps a second copy of the
// wire types out of this module.
package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Errors a [Runtime] reports by identity rather than by message, mirroring the
// symbolic names the runtime protocol uses for the same conditions. Commands
// branch on these; nothing branches on error text.
var (
	ErrSessionNotFound     = errors.New("session not found")
	ErrRunNotFound         = errors.New("run not found")
	ErrInterruptNotOpen    = errors.New("interrupt not open")
	ErrStaleSegment        = errors.New("stale segment")
	ErrRunWaiting          = errors.New("run is waiting")
	ErrRunFinished         = errors.New("run is finished")
	ErrReplayCursorInvalid = errors.New("event replay cursor is invalid")
	ErrReplayUnavailable   = errors.New("event replay unavailable")
	ErrSessionHasActiveRun = errors.New("session has an active run")
	ErrSessionBusy         = errors.New("session is busy")
	ErrRevisionConflict    = errors.New("revision conflict")
	ErrEventConflict       = errors.New("event identity conflict")
	ErrDisconnected        = errors.New("runtime disconnected")
	ErrIncompatibleRuntime = errors.New("runtime protocol is incompatible")
)

// BlockKind names what a transcript block is. The set is closed: an item a
// [Runtime] implementation cannot classify becomes a [BlockNotice], never a new
// kind invented at the render site.
type BlockKind string

const (
	BlockUser      BlockKind = "user"
	BlockAssistant BlockKind = "assistant"
	BlockReasoning BlockKind = "reasoning"
	BlockQuestion  BlockKind = "question"
	BlockTool      BlockKind = "tool"
	BlockNotice    BlockKind = "notice"
	BlockError     BlockKind = "error"
)

// BlockStatus is the durable lifecycle of a transcript item. Incomplete is a
// terminal item-level failure; it does not imply that the owning Run failed.
type BlockStatus string

const (
	BlockStatusRunning    BlockStatus = "running"
	BlockStatusCompleted  BlockStatus = "completed"
	BlockStatusIncomplete BlockStatus = "incomplete"
)

// Block is one renderable unit of a transcript. RunID preserves the durable
// item's provenance across cold reads; ID identifies the item within that Run.
type Block struct {
	ID        string
	RunID     string
	Status    BlockStatus
	Kind      BlockKind
	CreatedAt time.Time
	Redacted  bool
	// DroppedMessages is meaningful only for a compaction notice.
	DroppedMessages int
	Attachments     []Attachment
	Images          []InlineImage
	// Text is the block's body. Assistant and reasoning bodies are markdown and
	// arrive in pieces (see [BlockDelta]). Tool deltas append to Tool.Output;
	// the remaining block kinds arrive whole.
	Text string
	// Question carries the durable prompt when Kind is [BlockQuestion]. Its
	// ItemID is the same identity as this block's ID.
	Question *Question
	// Tool carries the call's projection when Kind is [BlockTool].
	Tool *ToolCall
}

// Clone returns a block with no mutable storage shared with the caller.
func (b Block) Clone() Block {
	b.Attachments = slices.Clone(b.Attachments)
	b.Images = cloneInlineImages(b.Images)
	if b.Question != nil {
		question := cloneQuestion(*b.Question)
		b.Question = &question
	}
	if b.Tool != nil {
		tool := b.Tool.Clone()
		b.Tool = &tool
	}
	return b
}

// Equal reports whether two blocks carry the same complete presentation fact.
// It deliberately compares nested projections by value so replay consistency
// does not depend on pointer identity or on a reflection-based struct layout.
func (b Block) Equal(other Block) bool {
	if b.ID != other.ID || b.RunID != other.RunID || b.Status != other.Status || b.Kind != other.Kind ||
		!b.CreatedAt.Equal(other.CreatedAt) || b.Redacted != other.Redacted || b.DroppedMessages != other.DroppedMessages ||
		b.Text != other.Text || !slices.Equal(b.Attachments, other.Attachments) || !equalInlineImages(b.Images, other.Images) {
		return false
	}
	if (b.Question == nil) != (other.Question == nil) ||
		(b.Question != nil && !b.Question.Equal(*other.Question)) {
		return false
	}
	return (b.Tool == nil) == (other.Tool == nil) &&
		(b.Tool == nil || b.Tool.Equal(*other.Tool))
}

// InlineImage is model-produced media embedded in an assistant message. It is
// deliberately separate from Attachment: attachments are workspace-backed
// authoring inputs, while an inline image is immutable output owned by the
// transcript itself.
type InlineImage struct {
	ID       string
	Name     string
	MIMEType string
	Data     []byte
}

func (image InlineImage) Clone() InlineImage {
	image.Data = bytes.Clone(image.Data)
	return image
}

func (image InlineImage) Equal(other InlineImage) bool {
	return image.ID == other.ID && image.Name == other.Name && image.MIMEType == other.MIMEType &&
		bytes.Equal(image.Data, other.Data)
}

func cloneInlineImages(images []InlineImage) []InlineImage {
	if images == nil {
		return nil
	}
	cloned := make([]InlineImage, len(images))
	for index, image := range images {
		cloned[index] = image.Clone()
	}
	return cloned
}

func equalInlineImages(left, right []InlineImage) bool {
	return slices.EqualFunc(left, right, func(left, right InlineImage) bool { return left.Equal(right) })
}

// ToolStatus is where a tool call is in its life.
type ToolStatus string

const (
	ToolRunning  ToolStatus = "running"
	ToolOK       ToolStatus = "ok"
	ToolError    ToolStatus = "error"
	ToolCanceled ToolStatus = "canceled"
)

// ToolSafetyClass is the runtime-classified mutation boundary of a tool call.
type ToolSafetyClass string

const (
	ToolSafetySafe    ToolSafetyClass = "safe"
	ToolSafetyWrite   ToolSafetyClass = "write"
	ToolSafetyExec    ToolSafetyClass = "exec"
	ToolSafetyNetwork ToolSafetyClass = "network"
)

// ToolKind is a terminal-relevant semantic category assigned by a runtime
// adapter. Delivery adapters switch on this closed projection, never on a
// provider's tool name.
type ToolKind string

const (
	ToolUnknown ToolKind = "unknown"
	ToolShell   ToolKind = "shell"
	ToolEdit    ToolKind = "edit"
	ToolRead    ToolKind = "read"
	ToolSearch  ToolKind = "search"
	ToolWeb     ToolKind = "web"
	ToolTask    ToolKind = "task"
)

// ToolCall is a tool invocation as the runtime adapter chose to present it.
//
// Every field is a projection the adapter already computed. Name preserves the
// provider-facing label for diagnostics; Kind and the structured fields below
// are the stable vocabulary renderers use. This keeps provider tool semantics in
// one adapter rather than rediscovering them in every UI.
type ToolCall struct {
	Kind       ToolKind
	Name       string
	Summary    string
	Status     ToolStatus
	Safety     ToolSafetyClass
	StartedAt  time.Time
	FinishedAt time.Time
	Command    string
	Path       string
	Query      string
	URL        string
	Output     string
	// ArgumentsJSON and ResultJSON preserve the complete, normalized JSON
	// values for generic presenters and machine consumers. Semantic fields such
	// as Command and Path remain the high-quality projection for known tools.
	ArgumentsJSON []byte
	ResultJSON    []byte
	// ProblemJSON retains the runtime's structured tool-level failure, including
	// documentation, retry guidance, and capability or field-level details.
	ProblemJSON []byte
	// Diff is a unified diff when the call changed files.
	Diff string
	// ExitCode is set for completed process-like tools. Nil distinguishes an
	// absent code from a successful zero.
	ExitCode *int
	// Duration is how long the call took, once it has finished.
	Duration time.Duration
}

func (t ToolCall) Clone() ToolCall {
	t.ArgumentsJSON = bytes.Clone(t.ArgumentsJSON)
	t.ResultJSON = bytes.Clone(t.ResultJSON)
	t.ProblemJSON = bytes.Clone(t.ProblemJSON)
	if t.ExitCode != nil {
		t.ExitCode = new(*t.ExitCode)
	}
	return t
}

// Equal reports whether two tool projections describe the same invocation
// state. An absent exit code remains distinct from an explicit successful zero.
func (t ToolCall) Equal(other ToolCall) bool {
	if t.Kind != other.Kind || t.Name != other.Name || t.Summary != other.Summary ||
		t.Status != other.Status || t.Safety != other.Safety || !t.StartedAt.Equal(other.StartedAt) ||
		!t.FinishedAt.Equal(other.FinishedAt) || t.Command != other.Command || t.Path != other.Path ||
		t.Query != other.Query || t.URL != other.URL || t.Output != other.Output ||
		!bytes.Equal(t.ArgumentsJSON, other.ArgumentsJSON) || !bytes.Equal(t.ResultJSON, other.ResultJSON) ||
		!bytes.Equal(t.ProblemJSON, other.ProblemJSON) ||
		t.Diff != other.Diff || t.Duration != other.Duration ||
		(t.ExitCode == nil) != (other.ExitCode == nil) {
		return false
	}
	return t.ExitCode == nil || *t.ExitCode == *other.ExitCode
}

func (t ToolCall) Validate() error {
	var problems []error
	switch t.Kind {
	case ToolUnknown, ToolShell, ToolEdit, ToolRead, ToolSearch, ToolWeb, ToolTask:
	default:
		problems = append(problems, fmt.Errorf("kind %q is invalid", t.Kind))
	}
	switch t.Status {
	case ToolRunning, ToolOK, ToolError, ToolCanceled:
	default:
		problems = append(problems, fmt.Errorf("status %q is invalid", t.Status))
	}
	switch t.Safety {
	case "", ToolSafetySafe, ToolSafetyWrite, ToolSafetyExec, ToolSafetyNetwork:
	default:
		problems = append(problems, fmt.Errorf("safety class %q is invalid", t.Safety))
	}
	if t.Kind == ToolUnknown && strings.TrimSpace(t.Name) == "" {
		problems = append(problems, errors.New("unknown tool has no provider name"))
	}
	if t.Status == ToolRunning && t.ExitCode != nil {
		problems = append(problems, errors.New("running tool has an exit code"))
	}
	if t.Duration < 0 {
		problems = append(problems, errors.New("tool duration is negative"))
	}
	if t.Status == ToolRunning && t.Duration != 0 {
		problems = append(problems, errors.New("running tool has a duration"))
	}
	if t.Status == ToolRunning && !t.FinishedAt.IsZero() {
		problems = append(problems, errors.New("running tool has a finish time"))
	}
	if !t.FinishedAt.IsZero() && t.StartedAt.IsZero() {
		problems = append(problems, errors.New("finished tool has no start time"))
	}
	if !t.FinishedAt.IsZero() && t.FinishedAt.Before(t.StartedAt) {
		problems = append(problems, errors.New("tool finish time precedes start time"))
	}
	if len(t.ArgumentsJSON) > 0 {
		var arguments map[string]any
		if !json.Valid(t.ArgumentsJSON) || json.Unmarshal(t.ArgumentsJSON, &arguments) != nil || arguments == nil {
			problems = append(problems, errors.New("arguments JSON is not an object"))
		}
	}
	if len(t.ResultJSON) > 0 && !json.Valid(t.ResultJSON) {
		problems = append(problems, errors.New("result JSON is invalid"))
	}
	if len(t.ProblemJSON) > 0 {
		var problem map[string]any
		if !json.Valid(t.ProblemJSON) || json.Unmarshal(t.ProblemJSON, &problem) != nil || problem == nil {
			problems = append(problems, errors.New("problem JSON is not an object"))
		}
		if t.Status != ToolError && t.Status != ToolCanceled {
			problems = append(problems, errors.New("successful or running tool carries a problem"))
		}
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("tool call: %w", err)
	}
	return nil
}

// PlanStatus is a plan item's state.
type PlanStatus string

const (
	PlanPending PlanStatus = "pending"
	PlanActive  PlanStatus = "active"
	PlanDone    PlanStatus = "done"
)

// PlanItem is one step of the run's plan.
type PlanItem struct {
	Title  string
	Status PlanStatus
}

// OutcomeStatus is how a run ended.
type OutcomeStatus string

const (
	OutcomeCompleted OutcomeStatus = "completed"
	OutcomeTimedOut  OutcomeStatus = "timedOut"
	OutcomeMaxSteps  OutcomeStatus = "maxSteps"
	OutcomeMaxBudget OutcomeStatus = "maxBudget"
	OutcomeCanceled  OutcomeStatus = "canceled"
	OutcomeFailed    OutcomeStatus = "failed"
	OutcomeLost      OutcomeStatus = "lost"
)

// Outcome is a finished run's verdict.
type Outcome struct {
	Status OutcomeStatus
	// Error carries the structured runtime problem's display text for failed,
	// timed-out, and lost outcomes.
	Error string
	// ProblemJSON retains the runtime's complete structured failure for machine
	// consumers and detailed presenters.
	ProblemJSON []byte
	// Detail explains a policy stop such as max steps, max budget, or cancel.
	Detail string
}

func (o Outcome) Clone() Outcome {
	o.ProblemJSON = bytes.Clone(o.ProblemJSON)
	return o
}

func (o Outcome) Equal(other Outcome) bool {
	return o.Status == other.Status && o.Error == other.Error && o.Detail == other.Detail &&
		bytes.Equal(o.ProblemJSON, other.ProblemJSON)
}
