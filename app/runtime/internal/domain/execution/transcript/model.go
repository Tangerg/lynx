package transcript

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// ErrIdentityConflict reports an attempt to reuse a durable transcript identity
// for a different owner. Item ids are bound to one Session+Run and Run ids to
// one Session for their entire lifetime; persistence must never re-parent them.
var ErrIdentityConflict = errors.New("transcript: identity conflict")

// UnknownMessageMark is the [Run.MessageMark] of a Run whose conversation
// watermark is not knowable yet: the count is captured when the Run finishes,
// and the chat log keeps growing until then. It is negative so it can never be
// mistaken for a real count — including an empty log's zero.
const UnknownMessageMark = -1

type Run struct {
	SessionID       string
	ID              string
	SpawnedByItemID string
	ModelSelection  modelref.Selection
	State           execution.RunState
	Outcome         *execution.Outcome
	Result          *RunResult
	Detail          string
	Interrupts      []Interrupt
	CreatedAt       time.Time
	FinishedAt      time.Time
	UpdatedAt       time.Time
	MessageMark     int
}

type RunResult struct {
	Usage    *Usage
	Steps    int
	Error    *Problem
	Duration time.Duration
}

type ModelUsage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	CostUSD          *float64
}

type Usage struct {
	ModelUsage
	ByModel map[string]ModelUsage
}

type ItemStatus uint8

const (
	ItemRunning ItemStatus = iota
	ItemCompleted
	ItemIncomplete
)

type ItemKind uint8

const (
	UserMessage ItemKind = iota
	AgentMessage
	Reasoning
	Plan
	QuestionItem
	ToolCall
	Compaction
)

type Item struct {
	SessionID string
	ID        string
	RunID     string
	Status    ItemStatus
	CreatedAt time.Time
	Kind      ItemKind

	Content     []ContentBlock
	Text        string
	Redacted    bool
	Steps       []PlanStep
	Question    *Question
	Tool        *ToolInvocation
	SafetyClass tool.SafetyClass
	Error       *Problem

	Summary         string
	DroppedMessages int
}

// SequencedItem pairs a history Item with its position in the session's durable
// append order — the total order a paged read continues along, and the only one
// that is exact: creation timestamps tie, and an imported transcript can carry
// backdated ones.
//
// The position sits beside the Item rather than inside it because the store
// assigns it when the Item lands: an Item on its way to being appended has no
// position, so a field for one inside the aggregate would be a zero every writer
// had to remember to ignore.
type SequencedItem struct {
	Sequence int64
	Item     Item
}

type ContentKind uint8

const (
	TextContent ContentKind = iota
	ImageContent
)

type ContentBlock struct {
	Kind ContentKind
	Text string
	Mime string
	Data string
}

type PlanStep struct {
	ID     string
	Title  string
	Status PlanStepStatus
}

// PlanStepStatus is the lifecycle state of a plan step.
type PlanStepStatus string

const (
	PlanStepPending   PlanStepStatus = "pending"
	PlanStepRunning   PlanStepStatus = "running"
	PlanStepCompleted PlanStepStatus = "completed"
	PlanStepFailed    PlanStepStatus = "failed"
)

type Question struct {
	Prompt string
	Fields []QuestionField
}

type QuestionField struct {
	Name     string
	Label    string
	Header   string
	Required bool
	Kind     QuestionFieldKind
	Options  []QuestionOption
	Multiple bool
}

type QuestionFieldKind uint8

const (
	QuestionText QuestionFieldKind = iota
	QuestionChoice
)

type QuestionOption struct {
	Label       string
	Description string
	Preview     string
}

type ToolInvocation struct {
	Name      string
	Arguments tool.Arguments
	Result    *tool.Result
	Offload   *offload.Ref
}

type ProblemScope uint8

const (
	RunProblem ProblemScope = iota
	ToolProblem
)

type ProblemKind uint8

const (
	InternalProblem ProblemKind = iota
	RunLostProblem
	AgentStuckProblem
	RateLimitedProblem
	InvalidAPIKeyProblem
	TimeoutProblem
	ProviderUnavailableProblem
	ProviderRejectedProblem
	DeniedByUserProblem
	ToolFailedProblem
)

// Problem classifies a failure. There is deliberately no transient/permanent
// flag: the three kinds that once set one are exactly the three that accept a
// RetryAfterSeconds, so the flag carried nothing the kind didn't already say,
// and a client that branches on it is branching on a derived value instead of
// the classification.
type Problem struct {
	Kind              ProblemKind
	Scope             ProblemScope
	Detail            string
	DocURL            string
	RetryAfterSeconds int
}

type InterruptKind uint8

const (
	ApprovalInterrupt InterruptKind = iota
	QuestionInterrupt
)

type Interrupt struct {
	ItemID   string
	Kind     InterruptKind
	Approval *Approval
	Question *Question
}

type Approval struct {
	Tool         ToolInvocation
	Risk         tool.RiskLevel
	Reason       string
	Rememberable bool
}

// Validate reports whether a Run's lifecycle facts agree with its state.
//
// A Run carries its terminal facts — outcome, result, detail, finish time, and
// message watermark — exactly when it has reached a terminal state, and carries
// none of them before. That equivalence is what lets the durable row keep them
// as plain columns with no separate "is there a result" flag: the state answers
// it. Both the store's write path and the portable-snapshot boundary check the
// rule here instead of restating it, so there is one place to read it.
func (run Run) Validate() error {
	switch {
	case run.ID == "":
		return errors.New("id is required")
	case run.SessionID == "":
		return errors.New("sessionId is required")
	case run.CreatedAt.IsZero():
		return errors.New("creation time is required")
	}
	if run.State.IsTerminal() {
		return run.validateTerminal()
	}
	return run.validateOpen()
}

func (run Run) validateOpen() error {
	switch {
	case run.Outcome != nil:
		return fmt.Errorf("%s run carries outcome %s", run.State, run.Outcome)
	case run.Result != nil:
		return fmt.Errorf("%s run carries a result", run.State)
	case run.Detail != "":
		return fmt.Errorf("%s run carries a terminal detail", run.State)
	case !run.FinishedAt.IsZero():
		return fmt.Errorf("%s run carries a finish time", run.State)
	case run.MessageMark != UnknownMessageMark:
		return fmt.Errorf("%s run carries message watermark %d", run.State, run.MessageMark)
	}
	// Being parked and holding open interrupts are the same fact seen twice: the
	// interrupts ARE what the run is parked on.
	if (run.State == execution.Interrupted) != (len(run.Interrupts) != 0) {
		return fmt.Errorf("%s run holds %d open interrupts", run.State, len(run.Interrupts))
	}
	return nil
}

func (run Run) validateTerminal() error {
	if run.Outcome == nil {
		return errors.New("terminal run has no outcome")
	}
	expected, ok := execution.Running.Terminate(*run.Outcome)
	if !ok || expected != run.State {
		return fmt.Errorf("state %s does not match outcome %s", run.State, run.Outcome)
	}
	if run.Result == nil {
		return errors.New("terminal run has no result")
	}
	if (*run.Outcome == execution.OutcomeError) != (run.Result.Error != nil) {
		return fmt.Errorf("error result does not match outcome %s", run.Outcome)
	}
	if err := run.Result.Validate(); err != nil {
		return err
	}
	switch {
	case run.FinishedAt.IsZero():
		return errors.New("terminal run has no finish time")
	case run.MessageMark < 0:
		return fmt.Errorf("terminal run has message watermark %d", run.MessageMark)
	case len(run.Interrupts) != 0:
		return fmt.Errorf("terminal run holds %d open interrupts", len(run.Interrupts))
	}
	return nil
}

// Validate reports whether the result accounting is internally consistent.
func (result *RunResult) Validate() error {
	if result == nil {
		return nil
	}
	if result.Steps < 0 || result.Duration < 0 {
		return errors.New("result accounting must not be negative")
	}
	if err := result.Usage.Validate(); err != nil {
		return err
	}
	return result.Error.ValidateFor(RunProblem)
}

// Validate reports whether the usage accounting is internally consistent.
func (usage *Usage) Validate() error {
	if usage == nil {
		return nil
	}
	if err := usage.ModelUsage.Validate(); err != nil {
		return err
	}
	for model, perModel := range usage.ByModel {
		if model == "" {
			return errors.New("contains an empty model id")
		}
		if err := perModel.Validate(); err != nil {
			return fmt.Errorf("model %q: %w", model, err)
		}
	}
	return nil
}

// Validate reports whether one model's usage accounting is internally consistent.
func (usage ModelUsage) Validate() error {
	if usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.CacheReadTokens < 0 || usage.CacheWriteTokens < 0 || usage.ReasoningTokens < 0 {
		return errors.New("token counts must not be negative")
	}
	if usage.CostUSD != nil && *usage.CostUSD < 0 {
		return errors.New("cost must not be negative")
	}
	return nil
}

// Validate reports whether the problem value is internally consistent.
func (problem *Problem) Validate() error {
	if problem == nil {
		return nil
	}
	switch problem.Scope {
	case RunProblem, ToolProblem:
	default:
		return fmt.Errorf("unknown scope %d", problem.Scope)
	}
	switch problem.Kind {
	case InternalProblem, RunLostProblem, AgentStuckProblem,
		RateLimitedProblem, InvalidAPIKeyProblem, TimeoutProblem,
		ProviderUnavailableProblem, ProviderRejectedProblem,
		DeniedByUserProblem, ToolFailedProblem:
	default:
		return fmt.Errorf("unknown kind %d", problem.Kind)
	}
	if problem.RetryAfterSeconds < 0 {
		return errors.New("retry delay must not be negative")
	}
	return nil
}

// ValidateFor reports whether a problem is valid for its owning aggregate slot.
func (problem *Problem) ValidateFor(scope ProblemScope) error {
	if problem == nil {
		return nil
	}
	if err := problem.Validate(); err != nil {
		return err
	}
	if problem.Scope != scope {
		return fmt.Errorf("scope %d, want %d", problem.Scope, scope)
	}
	return nil
}

// Validate reports whether the content block has the shape required by its kind.
func (block ContentBlock) Validate() error {
	switch block.Kind {
	case TextContent:
		if block.Mime != "" || block.Data != "" {
			return errors.New("text content cannot carry mime or data")
		}
	case ImageContent:
		if block.Mime == "" || block.Data == "" {
			return errors.New("image content requires mime and data")
		}
		if block.Text != "" {
			return errors.New("image content cannot carry text")
		}
	default:
		return fmt.Errorf("unknown content kind %d", block.Kind)
	}
	return nil
}

// Validate reports whether the plan-step lifecycle is known.
func (step PlanStep) Validate() error {
	if !step.Status.Valid() {
		return fmt.Errorf("unknown status %q", step.Status)
	}
	return nil
}

// Valid reports whether the status is a defined plan-step lifecycle state.
func (status PlanStepStatus) Valid() bool {
	switch status {
	case PlanStepPending, PlanStepRunning, PlanStepCompleted, PlanStepFailed:
		return true
	default:
		return false
	}
}

// Validate reports whether the question can be rendered and answered unambiguously.
func (question Question) Validate() error {
	seen := make(map[string]struct{}, len(question.Fields))
	for index, field := range question.Fields {
		if field.Name == "" {
			return fmt.Errorf("question field %d name is required", index)
		}
		if _, duplicate := seen[field.Name]; duplicate {
			return fmt.Errorf("question field %q is duplicated", field.Name)
		}
		seen[field.Name] = struct{}{}
		switch field.Kind {
		case QuestionText:
			if len(field.Options) != 0 || field.Multiple {
				return fmt.Errorf("text question field %q cannot carry options or multiple", field.Name)
			}
		case QuestionChoice:
		default:
			return fmt.Errorf("question field %q has unknown kind %d", field.Name, field.Kind)
		}
		for optionIndex, option := range field.Options {
			if option.Label == "" {
				return fmt.Errorf("question field %q option %d label is required", field.Name, optionIndex)
			}
		}
	}
	return nil
}

// Validate reports whether the item has exactly the payload allowed by its kind.
func (item Item) Validate() error {
	switch item.Status {
	case ItemRunning, ItemCompleted, ItemIncomplete:
	default:
		return fmt.Errorf("unknown status %d", item.Status)
	}
	if item.DroppedMessages < 0 {
		return errors.New("dropped messages must not be negative")
	}
	if err := item.Error.ValidateFor(ToolProblem); err != nil {
		return fmt.Errorf("tool problem: %w", err)
	}
	for index, block := range item.Content {
		if err := block.Validate(); err != nil {
			return fmt.Errorf("content %d: %w", index, err)
		}
	}
	for index, step := range item.Steps {
		if err := step.Validate(); err != nil {
			return fmt.Errorf("plan step %d: %w", index, err)
		}
	}
	if item.Question != nil {
		if err := item.Question.Validate(); err != nil {
			return err
		}
	}
	if item.Tool != nil && item.Tool.Name == "" {
		return errors.New("tool name is required")
	}

	switch item.Kind {
	case UserMessage, AgentMessage:
		return item.rejectPayload(
			payloadField{"text", item.Text != ""}, payloadField{"redacted", item.Redacted},
			payloadField{"steps", len(item.Steps) != 0}, payloadField{"question", item.Question != nil},
			payloadField{"tool", item.Tool != nil}, payloadField{"safetyClass", item.SafetyClass != ""},
			payloadField{"error", item.Error != nil}, payloadField{"summary", item.Summary != ""},
			payloadField{"droppedMessages", item.DroppedMessages != 0},
		)
	case Reasoning:
		return item.rejectPayload(
			payloadField{"content", len(item.Content) != 0}, payloadField{"steps", len(item.Steps) != 0},
			payloadField{"question", item.Question != nil}, payloadField{"tool", item.Tool != nil},
			payloadField{"safetyClass", item.SafetyClass != ""}, payloadField{"error", item.Error != nil},
			payloadField{"summary", item.Summary != ""}, payloadField{"droppedMessages", item.DroppedMessages != 0},
		)
	case Plan:
		return item.rejectPayload(
			payloadField{"content", len(item.Content) != 0}, payloadField{"text", item.Text != ""},
			payloadField{"redacted", item.Redacted}, payloadField{"question", item.Question != nil},
			payloadField{"tool", item.Tool != nil}, payloadField{"safetyClass", item.SafetyClass != ""},
			payloadField{"error", item.Error != nil}, payloadField{"summary", item.Summary != ""},
			payloadField{"droppedMessages", item.DroppedMessages != 0},
		)
	case QuestionItem:
		if item.Question == nil {
			return errors.New("question is required")
		}
		return item.rejectPayload(
			payloadField{"content", len(item.Content) != 0}, payloadField{"text", item.Text != ""},
			payloadField{"redacted", item.Redacted}, payloadField{"steps", len(item.Steps) != 0},
			payloadField{"tool", item.Tool != nil}, payloadField{"safetyClass", item.SafetyClass != ""},
			payloadField{"error", item.Error != nil}, payloadField{"summary", item.Summary != ""},
			payloadField{"droppedMessages", item.DroppedMessages != 0},
		)
	case ToolCall:
		if item.Tool == nil {
			return errors.New("tool invocation is required")
		}
		if item.SafetyClass != "" && !item.SafetyClass.Valid() {
			return fmt.Errorf("unknown safety class %q", item.SafetyClass)
		}
		return item.rejectPayload(
			payloadField{"content", len(item.Content) != 0}, payloadField{"text", item.Text != ""},
			payloadField{"redacted", item.Redacted}, payloadField{"steps", len(item.Steps) != 0},
			payloadField{"question", item.Question != nil}, payloadField{"summary", item.Summary != ""},
			payloadField{"droppedMessages", item.DroppedMessages != 0},
		)
	case Compaction:
		return item.rejectPayload(
			payloadField{"content", len(item.Content) != 0}, payloadField{"text", item.Text != ""},
			payloadField{"redacted", item.Redacted}, payloadField{"steps", len(item.Steps) != 0},
			payloadField{"question", item.Question != nil}, payloadField{"tool", item.Tool != nil},
			payloadField{"safetyClass", item.SafetyClass != ""}, payloadField{"error", item.Error != nil},
		)
	default:
		return fmt.Errorf("unknown kind %d", item.Kind)
	}
}

type payloadField struct {
	name    string
	present bool
}

func (item Item) rejectPayload(fields ...payloadField) error {
	for _, field := range fields {
		if field.present {
			return fmt.Errorf("%s is not valid for item kind %d", field.name, item.Kind)
		}
	}
	return nil
}
