package transcript

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
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

// Run is one execution of a Session's agent, from admission to a terminal
// state. Its facts are split by the question they answer: [Run.Outcome] /
// [Run.Detail] / [Run.Error] say WHY it stopped and exist only once it has,
// while [Run.Metrics] says HOW MUCH it has consumed and exists from the first
// segment onward. A single carrier for both made "how much did this cost" a
// question only a finished Run could answer.
type Run struct {
	SessionID       string
	ID              string
	SpawnedByItemID string
	ParentRunID     string
	RootRunID       string
	ModelSelection  modelref.Selection
	// GoalLeaseID is immutable admission provenance for a root autonomous Run.
	// It remains private to autonomous-goal accounting and is deliberately omitted
	// from portable session snapshots, which cannot resurrect a live goal lease.
	GoalLeaseID string
	State       rundomain.State
	// ActiveSegmentID is the segment currently executing. It exists exactly while
	// the Run is Running: it is established, replaced and cleared in the same
	// transaction as the state, so a Run's position and the segment driving it can
	// never disagree.
	ActiveSegmentID string
	Outcome         *rundomain.Outcome
	Detail          string
	// Error explains an OutcomeFailed terminal, and is absent for every other
	// state and outcome. It travels with the outcome rather than with the
	// accounting because it is part of why the Run stopped.
	Error *Problem
	// Metrics is the Run's cumulative consumption as of this record — summed
	// across every segment, so a resumed Run reports the whole Run and not just
	// its last continuation. Values never decrease from one committed record to
	// the next.
	Metrics RunMetrics
	// Limits is the allowance actually in force for this Run, frozen at
	// admission. It is durable rather than a per-request echo because a resume
	// and restart recovery have to apply the same caps the first segment
	// did.
	Limits rundomain.Limits
	// Capabilities is the optional behavior enabled at admission and retained for
	// the Run's whole life. A continuation may exercise the same child-Run and
	// interrupt behavior; it cannot renegotiate either.
	Capabilities rundomain.Capabilities
	Interrupts   []Interrupt
	CreatedAt    time.Time
	FinishedAt   time.Time
	UpdatedAt    time.Time
	MessageMark  int
}

// RunMetrics is what a Run has consumed, accumulated over all of its segments.
// Usage is absent until the provider reports any; once reported it stays.
type RunMetrics struct {
	Usage *Usage
	Steps int
	// ActiveDuration is time spent executing. Waiting on a person is not active,
	// so a Run parked overnight accrues nothing while parked — which is why this
	// is a sum of segment durations rather than FinishedAt minus CreatedAt.
	ActiveDuration time.Duration
}

// Equal reports whether two snapshots contain the same cumulative accounting
// fact. Nil Usage is intentionally distinct from a reported zero, while nil and
// empty per-model maps are the same set.
func (m RunMetrics) Equal(other RunMetrics) bool {
	if m.Steps != other.Steps || m.ActiveDuration != other.ActiveDuration {
		return false
	}
	return equalUsage(m.Usage, other.Usage)
}

func equalUsage(left, right *Usage) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	if !equalModelUsage(left.ModelUsage, right.ModelUsage) || len(left.ByModel) != len(right.ByModel) {
		return false
	}
	for model, usage := range left.ByModel {
		other, found := right.ByModel[model]
		if !found || !equalModelUsage(usage, other) {
			return false
		}
	}
	return true
}

func equalModelUsage(left, right ModelUsage) bool {
	if left.InputTokens != right.InputTokens ||
		left.OutputTokens != right.OutputTokens ||
		left.CacheReadTokens != right.CacheReadTokens ||
		left.CacheWriteTokens != right.CacheWriteTokens ||
		left.ReasoningTokens != right.ReasoningTokens {
		return false
	}
	if left.CostUSD == nil || right.CostUSD == nil {
		return left.CostUSD == nil && right.CostUSD == nil
	}
	return *left.CostUSD == *right.CostUSD
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
	// ItemKind values name the closed semantic variants of a transcript item.
	// Durable codecs choose and validate their own discriminants.
	UserMessage  ItemKind = 0
	AgentMessage ItemKind = 1
	Reasoning    ItemKind = 2
	QuestionItem ItemKind = 4
	ToolCall     ItemKind = 5
	Compaction   ItemKind = 6
)

type Item struct {
	SessionID  string
	ID         string
	RunID      string
	Status     ItemStatus
	OccurredAt time.Time
	// FinishedAt is the terminal boundary of a ToolCall. Tool execution starts
	// at OccurredAt; other Item kinds do not use this field.
	FinishedAt time.Time
	Kind       ItemKind

	Content     []ContentBlock
	Text        string
	Redacted    bool
	Question    *Question
	Tool        *ToolInvocation
	SafetyClass tool.SafetyClass
	Error       *Problem

	Summary         string
	DroppedMessages int
}

// SequencedItem pairs a history Item with its position in the session's durable
// append order — the total order a paged read continues along, and the only one
// that is exact: occurrence timestamps tie, and an imported transcript can carry
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

// SequenceOrder is the direction a paged read walks the durable sequence. Both
// directions are the SAME total order read from opposite ends — never a different
// sort — so a page is exact either way and the two cannot disagree about which item
// comes first.
type SequenceOrder string

const (
	// OldestFirst replays the session the way it happened, which is what folding it
	// back into state requires.
	OldestFirst SequenceOrder = "oldest"
	// NewestFirst reaches the tail without walking everything before it, which is
	// what showing a long session's last screen requires.
	NewestFirst SequenceOrder = "newest"
)

// Valid reports whether o names one of the two directions through the durable
// transcript sequence.
func (o SequenceOrder) Valid() bool {
	return o == OldestFirst || o == NewestFirst
}

// Validate rejects a direction that cannot define cursor and SQL ordering.
func (o SequenceOrder) Validate() error {
	if !o.Valid() {
		return fmt.Errorf("transcript: invalid sequence order %q", o)
	}
	return nil
}

func (o SequenceOrder) String() string { return string(o) }

type ContentKind uint8

const (
	TextContent ContentKind = iota
	ImageContent
)

type ContentBlock struct {
	Kind      ContentKind
	Text      string
	MediaType string
	Bytes     []byte
}

// Clone returns an ownership-isolated content value.
func (block ContentBlock) Clone() ContentBlock {
	block.Bytes = slices.Clone(block.Bytes)
	return block
}

// CloneContent returns an ownership-isolated sequence of content blocks.
func CloneContent(blocks []ContentBlock) []ContentBlock {
	cloned := make([]ContentBlock, len(blocks))
	for index, block := range blocks {
		cloned[index] = block.Clone()
	}
	return cloned
}

type Question struct {
	Fields []QuestionField
}

type QuestionField struct {
	Prompt      string
	Header      string
	Kind        QuestionFieldKind
	Options     []QuestionOption
	Multiple    bool
	AllowCustom bool
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
	Offload   *toolresult.Ref
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
	ChildRunCanceledProblem
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

// Interrupt is one thing a person has to answer before execution continues.
type Interrupt struct {
	ItemID string
	// ItemOccurredAt preserves the identity timestamp of the Running Item this
	// interrupt references. A continuation completes or reopens that same Item;
	// it must not manufacture a new occurrence when the Run resumes.
	ItemOccurredAt time.Time
	// RunID is the Run that RAISED this interrupt — not the Run that owns the set
	// it belongs to. The two are the same only while a tree is a single Run: a set
	// is owned by the root and consumed as a whole, while each interrupt was raised
	// somewhere in that tree and is answered in the context of the Run that asked.
	//
	// It is recorded rather than derived, because the derivation stops working
	// exactly when it starts to matter.
	RunID    string
	Kind     interrupt.Kind
	Approval *Approval
	Question *Question
}

type Approval struct {
	Tool         ToolInvocation
	Risk         tool.RiskLevel
	Reason       string
	Rememberable bool
}

// Validate reports whether an approval describes one pending tool invocation.
// A result or offload reference would mean the invocation already ran, while a
// missing risk cannot support an informed decision.
func (approval Approval) Validate() error {
	switch {
	case strings.TrimSpace(approval.Tool.Name) == "":
		return errors.New("approval tool name is required")
	case approval.Tool.Name != strings.TrimSpace(approval.Tool.Name):
		return errors.New("approval tool name has surrounding whitespace")
	case approval.Tool.Result != nil:
		return errors.New("approval tool must not carry a result")
	case approval.Tool.Offload != nil:
		return errors.New("approval tool must not carry an offload reference")
	case !approval.Risk.Valid():
		return fmt.Errorf("approval has unknown risk %q", approval.Risk)
	}
	return nil
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
	if err := run.Lineage().Validate(run.ID); err != nil {
		return err
	}
	if err := run.ModelSelection.Validate(); err != nil {
		return fmt.Errorf("model selection: %w", err)
	}
	if run.GoalLeaseID != strings.TrimSpace(run.GoalLeaseID) {
		return errors.New("goal lease ID has surrounding whitespace")
	}
	if run.Lineage().IsChild() && run.GoalLeaseID != "" {
		return errors.New("child run carries a root goal lease")
	}
	// A Run is Running exactly while a segment drives it. Both halves matter: a
	// running Run with no segment cannot be attached to, and a parked or finished
	// one still naming a segment would have a client attach to a stream that ended.
	if (run.State == rundomain.Running) != (run.ActiveSegmentID != "") {
		return fmt.Errorf("%s run has active segment %q", run.State, run.ActiveSegmentID)
	}
	// Accounting is checked for every state, not only the terminal one: metrics
	// now accrue from the first segment, so a nonsense value can be committed long
	// before the Run ends.
	if err := run.Metrics.Validate(); err != nil {
		return err
	}
	if err := run.Limits.Validate(); err != nil {
		return err
	}
	if err := run.Capabilities.Validate(); err != nil {
		return err
	}
	if run.State.IsTerminal() {
		return run.validateTerminal()
	}
	return run.validateOpen()
}

// Lineage returns the Run's immutable root/child identity as one value for
// validation and tree routing.
func (run Run) Lineage() rundomain.Lineage {
	return rundomain.Lineage{
		SpawnedByItemID: run.SpawnedByItemID,
		ParentRunID:     run.ParentRunID,
		RootRunID:       run.RootRunID,
	}
}

func (run Run) validateOpen() error {
	switch {
	case run.Outcome != nil:
		return fmt.Errorf("%s run carries outcome %s", run.State, run.Outcome)
	case run.Error != nil:
		return fmt.Errorf("%s run carries a failure", run.State)
	case run.Detail != "":
		return fmt.Errorf("%s run carries a terminal detail", run.State)
	case !run.FinishedAt.IsZero():
		return fmt.Errorf("%s run carries a finish time", run.State)
	case run.MessageMark != UnknownMessageMark:
		return fmt.Errorf("%s run carries message watermark %d", run.State, run.MessageMark)
	}
	// A Running Run cannot own an open human question. An Waiting Run may
	// carry direct interrupts, or none when another Run in its tree raised the
	// barrier and this Run was suspended with it.
	if run.State == rundomain.Running && len(run.Interrupts) != 0 {
		return fmt.Errorf("running run holds %d open interrupts", len(run.Interrupts))
	}
	return nil
}

func (run Run) validateTerminal() error {
	if run.Outcome == nil {
		return errors.New("terminal run has no outcome")
	}
	expected, ok := rundomain.Running.Terminate(*run.Outcome)
	if !ok || expected != run.State {
		return fmt.Errorf("state %s does not match outcome %s", run.State, run.Outcome)
	}
	switch *run.Outcome {
	case rundomain.OutcomeFailed:
		if run.Error == nil {
			return errors.New("failed run has no problem")
		}
	case rundomain.OutcomeTimedOut:
		if run.Error == nil || run.Error.Kind != TimeoutProblem {
			return errors.New("timed-out run has no timeout problem")
		}
	case rundomain.OutcomeLost:
		if run.Error == nil || run.Error.Kind != RunLostProblem {
			return errors.New("lost run has no run-lost problem")
		}
	default:
		if run.Error != nil {
			return fmt.Errorf("outcome %s carries a problem", run.Outcome)
		}
	}
	if err := run.Error.ValidateFor(RunProblem); err != nil {
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

// Validate reports whether the accumulated consumption is internally consistent.
func (m RunMetrics) Validate() error {
	if m.Steps < 0 || m.ActiveDuration < 0 {
		return errors.New("run metrics must not be negative")
	}
	return m.Usage.Validate()
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
	if usage.CostUSD != nil && (*usage.CostUSD < 0 || math.IsNaN(*usage.CostUSD) || math.IsInf(*usage.CostUSD, 0)) {
		return errors.New("cost must be finite and non-negative")
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
		DeniedByUserProblem, ToolFailedProblem, ChildRunCanceledProblem:
	default:
		return fmt.Errorf("unknown kind %d", problem.Kind)
	}
	if problem.RetryAfterSeconds < 0 {
		return errors.New("retry delay must not be negative")
	}
	if problem.RetryAfterSeconds > 0 {
		switch problem.Kind {
		case RateLimitedProblem, TimeoutProblem, ProviderUnavailableProblem:
		default:
			return fmt.Errorf("problem kind %d cannot carry a retry delay", problem.Kind)
		}
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
		if block.Text == "" {
			return errors.New("text content requires text")
		}
		if block.MediaType != "" || len(block.Bytes) != 0 {
			return errors.New("text content cannot carry media")
		}
	case ImageContent:
		if !strings.HasPrefix(block.MediaType, "image/") || len(block.Bytes) == 0 {
			return errors.New("image content requires an image media type and bytes")
		}
		if block.Text != "" {
			return errors.New("image content cannot carry text")
		}
	default:
		return fmt.Errorf("unknown content kind %d", block.Kind)
	}
	return nil
}

// Validate reports whether the question can be rendered and answered unambiguously.
func (question Question) Validate() error {
	if len(question.Fields) == 0 {
		return errors.New("question requires at least one field")
	}
	for index, field := range question.Fields {
		if strings.TrimSpace(field.Prompt) == "" {
			return fmt.Errorf("question field %d prompt is required", index)
		}
		if utf8.RuneCountInString(field.Header) > 12 {
			return fmt.Errorf("question field %d header must be at most 12 characters", index)
		}
		switch field.Kind {
		case QuestionText:
			if len(field.Options) != 0 || field.Multiple || field.AllowCustom {
				return fmt.Errorf("text question field %d cannot carry choice settings", index)
			}
		case QuestionChoice:
			if len(field.Options) < 2 {
				return fmt.Errorf("choice question field %d requires at least two options", index)
			}
		default:
			return fmt.Errorf("question field %d has unknown kind %d", index, field.Kind)
		}
		seenOptions := make(map[string]struct{}, len(field.Options))
		for optionIndex, option := range field.Options {
			label := strings.TrimSpace(option.Label)
			if label == "" {
				return fmt.Errorf("question field %d option %d label is required", index, optionIndex)
			}
			if label != option.Label {
				return fmt.Errorf("question field %d option %d label has surrounding whitespace", index, optionIndex)
			}
			if _, duplicate := seenOptions[label]; duplicate {
				return fmt.Errorf("question field %d repeats option %q", index, label)
			}
			seenOptions[label] = struct{}{}
		}
	}
	return nil
}

// Validate reports whether the item has exactly the payload allowed by its kind.
func (item Item) Validate() error {
	if err := item.validateEnvelope(); err != nil {
		return err
	}
	return item.validateKindPayload()
}

func (item Item) validateEnvelope() error {
	switch item.Status {
	case ItemRunning, ItemCompleted, ItemIncomplete:
	default:
		return fmt.Errorf("unknown status %d", item.Status)
	}
	if item.DroppedMessages < 0 {
		return errors.New("dropped messages must not be negative")
	}
	if item.OccurredAt.IsZero() {
		return errors.New("occurred at is required")
	}
	if err := item.Error.ValidateFor(ToolProblem); err != nil {
		return fmt.Errorf("tool problem: %w", err)
	}
	for index, block := range item.Content {
		if err := block.Validate(); err != nil {
			return fmt.Errorf("content %d: %w", index, err)
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
	if item.Kind != ToolCall && !item.FinishedAt.IsZero() {
		return errors.New("finished at is only valid for tool calls")
	}
	return nil
}

func (item Item) validateKindPayload() error {
	switch item.Kind {
	case UserMessage, AgentMessage:
		return item.rejectDisallowedPayload(
			itemPayloadField{"text", item.Text != ""}, itemPayloadField{"redacted", item.Redacted},
			itemPayloadField{"question", item.Question != nil},
			itemPayloadField{"tool", item.Tool != nil}, itemPayloadField{"safetyClass", item.SafetyClass != ""},
			itemPayloadField{"error", item.Error != nil}, itemPayloadField{"summary", item.Summary != ""},
			itemPayloadField{"droppedMessages", item.DroppedMessages != 0},
		)
	case Reasoning:
		return item.rejectDisallowedPayload(
			itemPayloadField{"content", len(item.Content) != 0},
			itemPayloadField{"question", item.Question != nil}, itemPayloadField{"tool", item.Tool != nil},
			itemPayloadField{"safetyClass", item.SafetyClass != ""}, itemPayloadField{"error", item.Error != nil},
			itemPayloadField{"summary", item.Summary != ""}, itemPayloadField{"droppedMessages", item.DroppedMessages != 0},
		)
	case QuestionItem:
		if item.Question == nil {
			return errors.New("question is required")
		}
		return item.rejectDisallowedPayload(
			itemPayloadField{"content", len(item.Content) != 0}, itemPayloadField{"text", item.Text != ""},
			itemPayloadField{"redacted", item.Redacted},
			itemPayloadField{"tool", item.Tool != nil}, itemPayloadField{"safetyClass", item.SafetyClass != ""},
			itemPayloadField{"error", item.Error != nil}, itemPayloadField{"summary", item.Summary != ""},
			itemPayloadField{"droppedMessages", item.DroppedMessages != 0},
		)
	case ToolCall:
		return item.validateToolCallPayload()
	case Compaction:
		return item.rejectDisallowedPayload(
			itemPayloadField{"content", len(item.Content) != 0}, itemPayloadField{"text", item.Text != ""},
			itemPayloadField{"redacted", item.Redacted},
			itemPayloadField{"question", item.Question != nil}, itemPayloadField{"tool", item.Tool != nil},
			itemPayloadField{"safetyClass", item.SafetyClass != ""}, itemPayloadField{"error", item.Error != nil},
		)
	default:
		return fmt.Errorf("unknown kind %d", item.Kind)
	}
}

func (item Item) validateToolCallPayload() error {
	if item.Tool == nil {
		return errors.New("tool invocation is required")
	}
	switch item.Status {
	case ItemRunning:
		if !item.FinishedAt.IsZero() {
			return errors.New("running tool call must not have a finish time")
		}
	case ItemCompleted, ItemIncomplete:
		if item.FinishedAt.IsZero() {
			return errors.New("terminal tool call finish time is required")
		}
		if item.FinishedAt.Before(item.OccurredAt) {
			return errors.New("tool call finish time precedes start time")
		}
	}
	if item.SafetyClass != "" && !item.SafetyClass.Valid() {
		return fmt.Errorf("unknown safety class %q", item.SafetyClass)
	}
	return item.rejectDisallowedPayload(
		itemPayloadField{"content", len(item.Content) != 0}, itemPayloadField{"text", item.Text != ""},
		itemPayloadField{"redacted", item.Redacted},
		itemPayloadField{"question", item.Question != nil}, itemPayloadField{"summary", item.Summary != ""},
		itemPayloadField{"droppedMessages", item.DroppedMessages != 0},
	)
}

type itemPayloadField struct {
	name    string
	present bool
}

func (item Item) rejectDisallowedPayload(fields ...itemPayloadField) error {
	for _, field := range fields {
		if field.present {
			return fmt.Errorf("%s is not valid for item kind %d", field.name, item.Kind)
		}
	}
	return nil
}
