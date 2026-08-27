package transcript

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/approval"
	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
	"github.com/Tangerg/scope/app/runtime/internal/domain/toolresult"
)

// ItemIdentity is the immutable ownership and occurrence identity shared by
// every transcript Item variant.
type ItemIdentity struct {
	SessionID  string
	RunID      string
	ItemID     string
	OccurredAt time.Time
}

// Validate reports whether the identity is complete and canonical.
func (i ItemIdentity) Validate() error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "Session ID", value: i.SessionID},
		{name: "Run ID", value: i.RunID},
		{name: "Item ID", value: i.ItemID},
	} {
		if strings.TrimSpace(field.value) == "" || field.value != strings.TrimSpace(field.value) {
			return fmt.Errorf("transcript: %s is required without surrounding whitespace", field.name)
		}
	}
	if i.OccurredAt.IsZero() {
		return errors.New("transcript: occurrence time is required")
	}
	return nil
}

// Item is one immutable, user-visible transcript fact. Only ToolCall has an
// internal status lifecycle. A Question prompt is complete when constructed and
// may later be replaced by the same fact enriched with its accepted answer.
type Item struct {
	identity          ItemIdentity
	status            ItemStatus
	finishedAt        time.Time
	executionDuration *time.Duration
	kind              ItemKind
	messagePhase      MessagePhase
	content           []ContentBlock
	text              string
	redacted          bool
	question          *Question
	tool              *ToolInvocation
	safetyClass       tool.SafetyClass
	approvalDecision  approval.Decision
	failure           *tool.Failure
	summary           string
	droppedMessages   int
}

// ItemSnapshot is the complete representation consumed by strict technical
// codecs. [RestoreItem] validates it; it is not a second mutation surface.
type ItemSnapshot struct {
	Identity          ItemIdentity
	Status            ItemStatus
	FinishedAt        time.Time
	ExecutionDuration *time.Duration
	Kind              ItemKind
	MessagePhase      MessagePhase
	Content           []ContentBlock
	Text              string
	Redacted          bool
	Question          *Question
	Tool              *ToolInvocation
	SafetyClass       tool.SafetyClass
	ApprovalDecision  approval.Decision
	Failure           *tool.Failure
	Summary           string
	DroppedMessages   int
}

// NewUserMessage constructs a complete user message.
func NewUserMessage(identity ItemIdentity, content []ContentBlock) (Item, error) {
	return newMessage(identity, UserMessage, content)
}

// NewAgentMessage constructs a complete agent message.
func NewAgentMessage(identity ItemIdentity, phase MessagePhase, content []ContentBlock) (Item, error) {
	return RestoreItem(ItemSnapshot{
		Identity: identity, Status: ItemCompleted, Kind: AgentMessage,
		MessagePhase: phase, Content: content,
	})
}

func newMessage(identity ItemIdentity, kind ItemKind, content []ContentBlock) (Item, error) {
	return RestoreItem(ItemSnapshot{
		Identity: identity, Status: ItemCompleted, Kind: kind, Content: content,
	})
}

// NewReasoning constructs a complete reasoning record.
func NewReasoning(identity ItemIdentity, text string, redacted bool) (Item, error) {
	return RestoreItem(ItemSnapshot{
		Identity: identity, Status: ItemCompleted, Kind: Reasoning,
		Text: text, Redacted: redacted,
	})
}

// NewQuestion constructs the complete prompt fact for one pending question.
// Whether it still awaits an answer belongs to the root-owned Pending set, not
// to a second lifecycle on the transcript Item.
func NewQuestion(identity ItemIdentity, question Question) (Item, error) {
	return RestoreItem(ItemSnapshot{
		Identity: identity, Status: ItemCompleted, Kind: QuestionItem, Question: &question,
	})
}

// NewToolCall constructs a running ToolCall. Its invocation must not already
// carry a result or offload reference.
func NewToolCall(identity ItemIdentity, invocation ToolInvocation, safetyClass tool.SafetyClass) (Item, error) {
	return RestoreItem(ItemSnapshot{
		Identity: identity, Status: ItemRunning, Kind: ToolCall,
		Tool: &invocation, SafetyClass: safetyClass,
	})
}

// NewCompaction constructs a complete compaction boundary.
func NewCompaction(identity ItemIdentity, summary string, droppedMessages int) (Item, error) {
	return RestoreItem(ItemSnapshot{
		Identity: identity, Status: ItemCompleted, Kind: Compaction,
		Summary: summary, DroppedMessages: droppedMessages,
	})
}

// RestoreItem rebuilds a complete Item from a technical record.
func RestoreItem(snapshot ItemSnapshot) (Item, error) {
	snapshot.Identity.OccurredAt = snapshot.Identity.OccurredAt.UTC()
	item := Item{
		identity: snapshot.Identity, status: snapshot.Status, finishedAt: snapshot.FinishedAt.UTC(),
		executionDuration: cloneDuration(snapshot.ExecutionDuration),
		kind:              snapshot.Kind, messagePhase: snapshot.MessagePhase,
		content: CloneContent(snapshot.Content), text: snapshot.Text,
		redacted: snapshot.Redacted, question: cloneQuestion(snapshot.Question),
		tool: cloneToolInvocation(snapshot.Tool), safetyClass: snapshot.SafetyClass,
		approvalDecision: snapshot.ApprovalDecision,
		failure:          cloneToolFailure(snapshot.Failure), summary: snapshot.Summary,
		droppedMessages: snapshot.DroppedMessages,
	}
	if err := item.Validate(); err != nil {
		return Item{}, err
	}
	return item, nil
}

// Snapshot returns a complete ownership-isolated representation.
func (i Item) Snapshot() ItemSnapshot {
	return ItemSnapshot{
		Identity: i.identity, Status: i.status, FinishedAt: i.finishedAt,
		ExecutionDuration: cloneDuration(i.executionDuration),
		Kind:              i.kind, MessagePhase: i.messagePhase,
		Content: CloneContent(i.content), Text: i.text,
		Redacted: i.redacted, Question: cloneQuestion(i.question),
		Tool: cloneToolInvocation(i.tool), SafetyClass: i.safetyClass,
		ApprovalDecision: i.approvalDecision,
		Failure:          cloneToolFailure(i.failure), Summary: i.summary,
		DroppedMessages: i.droppedMessages,
	}
}

// Fork derives a terminal historical Item for a child Session under fresh
// ownership identities. Semantic content and lifecycle times are preserved. An
// existing offloaded Tool result must be remapped together with its blob; the
// method neither introduces nor removes offloading.
func (i Item) Fork(sessionID, runID, itemID string, offload *toolresult.Ref) (Item, error) {
	if i.status == ItemRunning {
		return Item{}, errors.New("transcript: only a terminal Item can be forked")
	}
	snapshot := i.Snapshot()
	snapshot.Identity.SessionID = sessionID
	snapshot.Identity.RunID = runID
	snapshot.Identity.ItemID = itemID
	sourceOffloaded := snapshot.Tool != nil && snapshot.Tool.Offload != nil
	if sourceOffloaded != (offload != nil) {
		return Item{}, errors.New("transcript: fork must preserve Tool result offloading")
	}
	if snapshot.Tool != nil {
		snapshot.Tool.Offload = offload
	}
	return RestoreItem(snapshot)
}

// CompleteToolCall settles a running ToolCall successfully with the exact
// execution interval reported by the Tool boundary.
func (i Item) CompleteToolCall(
	invocation ToolInvocation,
	executionStartedAt time.Time,
	finishedAt time.Time,
) (Item, error) {
	return i.settleToolCall(invocation, nil, ItemCompleted, executionStartedAt, finishedAt)
}

// FailToolCall settles a started ToolCall with a classified failure and the
// exact execution interval reported by the Tool boundary.
func (i Item) FailToolCall(
	invocation ToolInvocation,
	failure tool.Failure,
	executionStartedAt time.Time,
	finishedAt time.Time,
) (Item, error) {
	return i.settleToolCall(invocation, &failure, ItemIncomplete, executionStartedAt, finishedAt)
}

// AbandonToolCall records that a running ToolCall never reached a conclusive
// result. Failure may be absent when only executor loss is known.
func (i Item) AbandonToolCall(failure *tool.Failure, finishedAt time.Time) (Item, error) {
	if i.tool == nil {
		return Item{}, errors.New("transcript: ToolCall invocation is absent")
	}
	return i.settleToolCall(*i.tool, failure, ItemIncomplete, time.Time{}, finishedAt)
}

// AbandonStartedToolCall records an inconclusive started Tool attempt whose
// executor boundary still supplied an exact execution interval.
func (i Item) AbandonStartedToolCall(
	failure *tool.Failure,
	executionStartedAt time.Time,
	finishedAt time.Time,
) (Item, error) {
	if i.tool == nil {
		return Item{}, errors.New("transcript: ToolCall invocation is absent")
	}
	return i.settleToolCall(*i.tool, failure, ItemIncomplete, executionStartedAt, finishedAt)
}

// ClassifyAbandonedToolCall attaches the causal failure that became known after
// an already-incomplete ToolCall was recorded. It is intentionally narrower
// than settlement: identity, invocation, status, and timing remain unchanged.
func (i Item) ClassifyAbandonedToolCall(failure tool.Failure) (Item, error) {
	if i.kind != ToolCall || i.status != ItemIncomplete {
		return Item{}, errors.New("transcript: only an incomplete ToolCall can be classified")
	}
	if i.failure != nil {
		return Item{}, errors.New("transcript: incomplete ToolCall already has a failure")
	}
	i.failure = cloneToolFailure(&failure)
	if err := i.Validate(); err != nil {
		return Item{}, err
	}
	return i, nil
}

// ResolveToolApproval records the exact human verdict accepted for a running
// ToolCall. The decision is a durable semantic fact on the invocation rather
// than a property of the current policy or its eventual execution outcome.
// Identity and lifecycle remain unchanged, and a second verdict is rejected.
func (i Item) ResolveToolApproval(decision approval.Decision) (Item, error) {
	if i.kind != ToolCall || i.status != ItemRunning || i.tool == nil {
		return Item{}, errors.New("transcript: only a running ToolCall can resolve approval")
	}
	if !decision.Valid() {
		return Item{}, fmt.Errorf("transcript: unknown Tool approval decision %q", decision)
	}
	if i.approvalDecision != "" {
		return Item{}, errors.New("transcript: ToolCall approval is already resolved")
	}
	i.approvalDecision = decision
	if err := i.Validate(); err != nil {
		return Item{}, err
	}
	return i, nil
}

func (i Item) settleToolCall(
	invocation ToolInvocation,
	failure *tool.Failure,
	status ItemStatus,
	executionStartedAt time.Time,
	finishedAt time.Time,
) (Item, error) {
	if i.kind != ToolCall || i.status != ItemRunning {
		return Item{}, errors.New("transcript: only a running ToolCall can settle")
	}
	if i.tool == nil || invocation.Name != i.tool.Name {
		return Item{}, errors.New("transcript: ToolCall settlement changes tool identity")
	}
	if finishedAt.IsZero() || finishedAt.Before(i.identity.OccurredAt) {
		return Item{}, errors.New("transcript: ToolCall finish time precedes its occurrence")
	}
	var executionDuration *time.Duration
	if !executionStartedAt.IsZero() {
		if executionStartedAt.Before(i.identity.OccurredAt) || finishedAt.Before(executionStartedAt) {
			return Item{}, errors.New("transcript: ToolCall execution interval is outside its lifecycle")
		}
		duration := finishedAt.Sub(executionStartedAt)
		executionDuration = &duration
	} else if status == ItemCompleted {
		return Item{}, errors.New("transcript: completed ToolCall requires an execution interval")
	}
	i.status, i.finishedAt = status, finishedAt.UTC()
	i.executionDuration = executionDuration
	i.tool, i.failure = cloneToolInvocation(&invocation), cloneToolFailure(failure)
	if err := i.Validate(); err != nil {
		return Item{}, err
	}
	return i, nil
}

// Validate reports whether the Item is one legal variant.
func (i Item) Validate() error {
	if err := i.identity.Validate(); err != nil {
		return err
	}
	if i.droppedMessages < 0 {
		return errors.New("transcript: dropped messages must not be negative")
	}
	for index, block := range i.content {
		if err := block.Validate(); err != nil {
			return fmt.Errorf("transcript: content %d: %w", index, err)
		}
	}
	switch i.kind {
	case UserMessage:
		if i.status != ItemCompleted || len(i.content) == 0 {
			return errors.New("transcript: message must be complete with content")
		}
	case AgentMessage:
		if i.status != ItemCompleted || len(i.content) == 0 {
			return errors.New("transcript: message must be complete with content")
		}
		if !i.messagePhase.Valid() {
			return fmt.Errorf("transcript: unknown AgentMessage phase %q", i.messagePhase)
		}
	case Reasoning:
		if i.status != ItemCompleted || i.text == "" {
			return errors.New("transcript: reasoning must be complete with text")
		}
	case QuestionItem:
		if i.status != ItemCompleted || i.question == nil {
			return errors.New("transcript: question must be a complete prompt")
		}
		if err := i.question.Validate(); err != nil {
			return err
		}
	case ToolCall:
		if err := i.validateToolCall(); err != nil {
			return err
		}
	case Compaction:
		if i.status != ItemCompleted {
			return errors.New("transcript: compaction must be complete")
		}
	default:
		return fmt.Errorf("transcript: unknown Item kind %q", i.kind)
	}
	return i.rejectDisallowedPayload()
}

func (i Item) validateToolCall() error {
	if i.tool == nil {
		return errors.New("transcript: ToolCall invocation is required")
	}
	if err := i.tool.Validate(i.status == ItemRunning); err != nil {
		return err
	}
	if i.safetyClass != "" && !i.safetyClass.Valid() {
		return fmt.Errorf("transcript: unknown Tool safety class %q", i.safetyClass)
	}
	if i.approvalDecision != "" && !i.approvalDecision.Valid() {
		return fmt.Errorf("transcript: unknown Tool approval decision %q", i.approvalDecision)
	}
	switch i.status {
	case ItemRunning:
		if !i.finishedAt.IsZero() || i.executionDuration != nil || i.failure != nil {
			return errors.New("transcript: running ToolCall carries terminal facts")
		}
	case ItemCompleted:
		if i.finishedAt.IsZero() || i.failure != nil {
			return errors.New("transcript: completed ToolCall has invalid terminal facts")
		}
	case ItemIncomplete:
		if i.finishedAt.IsZero() {
			return errors.New("transcript: incomplete ToolCall has no finish time")
		}
	default:
		return fmt.Errorf("transcript: unknown Item status %q", i.status)
	}
	if !i.finishedAt.IsZero() && i.finishedAt.Before(i.identity.OccurredAt) {
		return errors.New("transcript: ToolCall finish time precedes its occurrence")
	}
	if i.executionDuration != nil {
		if *i.executionDuration < 0 {
			return errors.New("transcript: ToolCall execution duration is negative")
		}
		if i.finishedAt.IsZero() || *i.executionDuration > i.finishedAt.Sub(i.identity.OccurredAt) {
			return errors.New("transcript: ToolCall execution duration exceeds its lifecycle")
		}
	}
	if i.failure != nil {
		if err := i.failure.Validate(); err != nil {
			return fmt.Errorf("transcript: Tool failure: %w", err)
		}
	}
	return nil
}

func (i Item) rejectDisallowedPayload() error {
	present := func(value bool, name string) error {
		if value {
			return fmt.Errorf("transcript: %s is not valid for Item kind %s", name, i.kind)
		}
		return nil
	}
	checks := []struct {
		value bool
		name  string
	}{
		{i.kind != UserMessage && i.kind != AgentMessage && len(i.content) != 0, "content"},
		{i.kind != AgentMessage && i.messagePhase != MessagePhaseNone, "AgentMessage phase"},
		{i.kind != Reasoning && i.text != "", "text"},
		{i.kind != Reasoning && i.redacted, "redacted"},
		{i.kind != QuestionItem && i.question != nil, "question"},
		{i.kind != ToolCall && i.tool != nil, "Tool invocation"},
		{i.kind != ToolCall && i.safetyClass != "", "Tool safety class"},
		{i.kind != ToolCall && i.approvalDecision != "", "Tool approval decision"},
		{i.kind != ToolCall && i.failure != nil, "Tool failure"},
		{i.kind != ToolCall && !i.finishedAt.IsZero(), "finish time"},
		{i.kind != ToolCall && i.executionDuration != nil, "execution duration"},
		{i.kind != Compaction && i.summary != "", "summary"},
		{i.kind != Compaction && i.droppedMessages != 0, "dropped messages"},
	}
	for _, check := range checks {
		if err := present(check.value, check.name); err != nil {
			return err
		}
	}
	return nil
}

// Validate reports whether an invocation has a canonical identity and result
// shape. pending requires the result and offload reference to be absent.
func (t ToolInvocation) Validate(pending bool) error {
	if strings.TrimSpace(t.Name) == "" || t.Name != strings.TrimSpace(t.Name) {
		return errors.New("transcript: Tool name is required without surrounding whitespace")
	}
	if pending && (t.Result != nil || t.Offload != nil) {
		return errors.New("transcript: pending Tool invocation carries a result")
	}
	if t.Offload != nil {
		if err := t.Offload.Validate(); err != nil {
			return fmt.Errorf("transcript: Tool offload: %w", err)
		}
		if t.Result == nil {
			return errors.New("transcript: offloaded Tool result has no preview")
		}
		if _, textual := t.Result.String(); !textual {
			return errors.New("transcript: offloaded Tool result preview is not text")
		}
	}
	return nil
}

func cloneToolInvocation(invocation *ToolInvocation) *ToolInvocation {
	if invocation == nil {
		return nil
	}
	copy := *invocation
	if invocation.Result != nil {
		result := *invocation.Result
		copy.Result = &result
	}
	if invocation.Offload != nil {
		offload := *invocation.Offload
		copy.Offload = &offload
	}
	return &copy
}

func cloneToolFailure(failure *tool.Failure) *tool.Failure {
	if failure == nil {
		return nil
	}
	copy := *failure
	return &copy
}

func cloneDuration(duration *time.Duration) *time.Duration {
	if duration == nil {
		return nil
	}
	copy := *duration
	return &copy
}

func cloneQuestion(question *Question) *Question {
	if question == nil {
		return nil
	}
	copy := Question{
		Fields:  make([]QuestionField, len(question.Fields)),
		Answers: cloneQuestionAnswers(question.Answers),
	}
	for index, field := range question.Fields {
		copy.Fields[index] = field
		copy.Fields[index].Options = append([]QuestionOption(nil), field.Options...)
	}
	return &copy
}

func cloneQuestionAnswers(answers [][]string) [][]string {
	if answers == nil {
		return nil
	}
	cloned := make([][]string, len(answers))
	for index, values := range answers {
		cloned[index] = append([]string(nil), values...)
	}
	return cloned
}

func (i Item) SessionID() string     { return i.identity.SessionID }
func (i Item) RunID() string         { return i.identity.RunID }
func (i Item) ID() string            { return i.identity.ItemID }
func (i Item) Status() ItemStatus    { return i.status }
func (i Item) OccurredAt() time.Time { return i.identity.OccurredAt }
func (i Item) FinishedAt() time.Time { return i.finishedAt }
func (i Item) ExecutionDuration() (time.Duration, bool) {
	if i.executionDuration == nil {
		return 0, false
	}
	return *i.executionDuration, true
}
func (i Item) Kind() ItemKind                      { return i.kind }
func (i Item) MessagePhase() MessagePhase          { return i.messagePhase }
func (i Item) Content() []ContentBlock             { return CloneContent(i.content) }
func (i Item) Text() string                        { return i.text }
func (i Item) Redacted() bool                      { return i.redacted }
func (i Item) SafetyClass() tool.SafetyClass       { return i.safetyClass }
func (i Item) ApprovalDecision() approval.Decision { return i.approvalDecision }
func (i Item) Summary() string                     { return i.summary }
func (i Item) DroppedMessages() int                { return i.droppedMessages }
func (i Item) Question() (Question, bool) {
	if i.question == nil {
		return Question{}, false
	}
	return *cloneQuestion(i.question), true
}
func (i Item) ToolInvocation() (ToolInvocation, bool) {
	if i.tool == nil {
		return ToolInvocation{}, false
	}
	return *cloneToolInvocation(i.tool), true
}

// AnswerQuestion enriches one complete Question prompt with the exact response
// accepted at the resume linearization point. Identity, occurrence, and Item
// status remain unchanged; a second answer is rejected.
func (i Item) AnswerQuestion(answers [][]string) (Item, error) {
	if i.kind != QuestionItem || i.question == nil || i.status != ItemCompleted {
		return Item{}, errors.New("transcript: only a complete Question can be answered")
	}
	if i.question.Answered() {
		return Item{}, errors.New("transcript: Question is already answered")
	}
	i.question = cloneQuestion(i.question)
	i.question.Answers = cloneQuestionAnswers(answers)
	if err := i.Validate(); err != nil {
		return Item{}, err
	}
	return i, nil
}
func (i Item) Failure() (tool.Failure, bool) {
	if i.failure == nil {
		return tool.Failure{}, false
	}
	return *cloneToolFailure(i.failure), true
}
