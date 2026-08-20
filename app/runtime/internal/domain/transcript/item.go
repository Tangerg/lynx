package transcript

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
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
func (identity ItemIdentity) Validate() error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "Session ID", value: identity.SessionID},
		{name: "Run ID", value: identity.RunID},
		{name: "Item ID", value: identity.ItemID},
	} {
		if strings.TrimSpace(field.value) == "" || field.value != strings.TrimSpace(field.value) {
			return fmt.Errorf("transcript: %s is required without surrounding whitespace", field.name)
		}
	}
	if identity.OccurredAt.IsZero() {
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
func (item Item) Snapshot() ItemSnapshot {
	return ItemSnapshot{
		Identity: item.identity, Status: item.status, FinishedAt: item.finishedAt,
		ExecutionDuration: cloneDuration(item.executionDuration),
		Kind:              item.kind, MessagePhase: item.messagePhase,
		Content: CloneContent(item.content), Text: item.text,
		Redacted: item.redacted, Question: cloneQuestion(item.question),
		Tool: cloneToolInvocation(item.tool), SafetyClass: item.safetyClass,
		ApprovalDecision: item.approvalDecision,
		Failure:          cloneToolFailure(item.failure), Summary: item.summary,
		DroppedMessages: item.droppedMessages,
	}
}

// Fork derives a terminal historical Item for a child Session under fresh
// ownership identities. Semantic content and lifecycle times are preserved. An
// existing offloaded Tool result must be remapped together with its blob; the
// method neither introduces nor removes offloading.
func (item Item) Fork(sessionID, runID, itemID string, offload *toolresult.Ref) (Item, error) {
	if item.status == ItemRunning {
		return Item{}, errors.New("transcript: only a terminal Item can be forked")
	}
	snapshot := item.Snapshot()
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
func (item Item) CompleteToolCall(
	invocation ToolInvocation,
	executionStartedAt time.Time,
	finishedAt time.Time,
) (Item, error) {
	return item.settleToolCall(invocation, nil, ItemCompleted, executionStartedAt, finishedAt)
}

// FailToolCall settles a started ToolCall with a classified failure and the
// exact execution interval reported by the Tool boundary.
func (item Item) FailToolCall(
	invocation ToolInvocation,
	failure tool.Failure,
	executionStartedAt time.Time,
	finishedAt time.Time,
) (Item, error) {
	return item.settleToolCall(invocation, &failure, ItemIncomplete, executionStartedAt, finishedAt)
}

// AbandonToolCall records that a running ToolCall never reached a conclusive
// result. Failure may be absent when only executor loss is known.
func (item Item) AbandonToolCall(failure *tool.Failure, finishedAt time.Time) (Item, error) {
	if item.tool == nil {
		return Item{}, errors.New("transcript: ToolCall invocation is absent")
	}
	return item.settleToolCall(*item.tool, failure, ItemIncomplete, time.Time{}, finishedAt)
}

// AbandonStartedToolCall records an inconclusive started Tool attempt whose
// executor boundary still supplied an exact execution interval.
func (item Item) AbandonStartedToolCall(
	failure *tool.Failure,
	executionStartedAt time.Time,
	finishedAt time.Time,
) (Item, error) {
	if item.tool == nil {
		return Item{}, errors.New("transcript: ToolCall invocation is absent")
	}
	return item.settleToolCall(*item.tool, failure, ItemIncomplete, executionStartedAt, finishedAt)
}

// ClassifyAbandonedToolCall attaches the causal failure that became known after
// an already-incomplete ToolCall was recorded. It is intentionally narrower
// than settlement: identity, invocation, status, and timing remain unchanged.
func (item Item) ClassifyAbandonedToolCall(failure tool.Failure) (Item, error) {
	if item.kind != ToolCall || item.status != ItemIncomplete {
		return Item{}, errors.New("transcript: only an incomplete ToolCall can be classified")
	}
	if item.failure != nil {
		return Item{}, errors.New("transcript: incomplete ToolCall already has a failure")
	}
	item.failure = cloneToolFailure(&failure)
	if err := item.Validate(); err != nil {
		return Item{}, err
	}
	return item, nil
}

// ResolveToolApproval records the exact human verdict accepted for a running
// ToolCall. The decision is a durable semantic fact on the invocation rather
// than a property of the current policy or its eventual execution outcome.
// Identity and lifecycle remain unchanged, and a second verdict is rejected.
func (item Item) ResolveToolApproval(decision approval.Decision) (Item, error) {
	if item.kind != ToolCall || item.status != ItemRunning || item.tool == nil {
		return Item{}, errors.New("transcript: only a running ToolCall can resolve approval")
	}
	if !decision.Valid() {
		return Item{}, fmt.Errorf("transcript: unknown Tool approval decision %q", decision)
	}
	if item.approvalDecision != "" {
		return Item{}, errors.New("transcript: ToolCall approval is already resolved")
	}
	item.approvalDecision = decision
	if err := item.Validate(); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (item Item) settleToolCall(
	invocation ToolInvocation,
	failure *tool.Failure,
	status ItemStatus,
	executionStartedAt time.Time,
	finishedAt time.Time,
) (Item, error) {
	if item.kind != ToolCall || item.status != ItemRunning {
		return Item{}, errors.New("transcript: only a running ToolCall can settle")
	}
	if item.tool == nil || invocation.Name != item.tool.Name {
		return Item{}, errors.New("transcript: ToolCall settlement changes tool identity")
	}
	if finishedAt.IsZero() || finishedAt.Before(item.identity.OccurredAt) {
		return Item{}, errors.New("transcript: ToolCall finish time precedes its occurrence")
	}
	var executionDuration *time.Duration
	if !executionStartedAt.IsZero() {
		if executionStartedAt.Before(item.identity.OccurredAt) || finishedAt.Before(executionStartedAt) {
			return Item{}, errors.New("transcript: ToolCall execution interval is outside its lifecycle")
		}
		duration := finishedAt.Sub(executionStartedAt)
		executionDuration = &duration
	} else if status == ItemCompleted {
		return Item{}, errors.New("transcript: completed ToolCall requires an execution interval")
	}
	item.status, item.finishedAt = status, finishedAt.UTC()
	item.executionDuration = executionDuration
	item.tool, item.failure = cloneToolInvocation(&invocation), cloneToolFailure(failure)
	if err := item.Validate(); err != nil {
		return Item{}, err
	}
	return item, nil
}

// Validate reports whether the Item is one legal variant.
func (item Item) Validate() error {
	if err := item.identity.Validate(); err != nil {
		return err
	}
	if item.droppedMessages < 0 {
		return errors.New("transcript: dropped messages must not be negative")
	}
	for index, block := range item.content {
		if err := block.Validate(); err != nil {
			return fmt.Errorf("transcript: content %d: %w", index, err)
		}
	}
	switch item.kind {
	case UserMessage:
		if item.status != ItemCompleted || len(item.content) == 0 {
			return errors.New("transcript: message must be complete with content")
		}
	case AgentMessage:
		if item.status != ItemCompleted || len(item.content) == 0 {
			return errors.New("transcript: message must be complete with content")
		}
		if !item.messagePhase.Valid() {
			return fmt.Errorf("transcript: unknown AgentMessage phase %d", item.messagePhase)
		}
	case Reasoning:
		if item.status != ItemCompleted || item.text == "" {
			return errors.New("transcript: reasoning must be complete with text")
		}
	case QuestionItem:
		if item.status != ItemCompleted || item.question == nil {
			return errors.New("transcript: question must be a complete prompt")
		}
		if err := item.question.Validate(); err != nil {
			return err
		}
	case ToolCall:
		if err := item.validateToolCall(); err != nil {
			return err
		}
	case Compaction:
		if item.status != ItemCompleted {
			return errors.New("transcript: compaction must be complete")
		}
	default:
		return fmt.Errorf("transcript: unknown Item kind %d", item.kind)
	}
	return item.rejectDisallowedPayload()
}

func (item Item) validateToolCall() error {
	if item.tool == nil {
		return errors.New("transcript: ToolCall invocation is required")
	}
	if err := item.tool.Validate(item.status == ItemRunning); err != nil {
		return err
	}
	if item.safetyClass != "" && !item.safetyClass.Valid() {
		return fmt.Errorf("transcript: unknown Tool safety class %q", item.safetyClass)
	}
	if item.approvalDecision != "" && !item.approvalDecision.Valid() {
		return fmt.Errorf("transcript: unknown Tool approval decision %q", item.approvalDecision)
	}
	switch item.status {
	case ItemRunning:
		if !item.finishedAt.IsZero() || item.executionDuration != nil || item.failure != nil {
			return errors.New("transcript: running ToolCall carries terminal facts")
		}
	case ItemCompleted:
		if item.finishedAt.IsZero() || item.failure != nil {
			return errors.New("transcript: completed ToolCall has invalid terminal facts")
		}
	case ItemIncomplete:
		if item.finishedAt.IsZero() {
			return errors.New("transcript: incomplete ToolCall has no finish time")
		}
	default:
		return fmt.Errorf("transcript: unknown Item status %d", item.status)
	}
	if !item.finishedAt.IsZero() && item.finishedAt.Before(item.identity.OccurredAt) {
		return errors.New("transcript: ToolCall finish time precedes its occurrence")
	}
	if item.executionDuration != nil {
		if *item.executionDuration < 0 {
			return errors.New("transcript: ToolCall execution duration is negative")
		}
		if item.finishedAt.IsZero() || *item.executionDuration > item.finishedAt.Sub(item.identity.OccurredAt) {
			return errors.New("transcript: ToolCall execution duration exceeds its lifecycle")
		}
	}
	if item.failure != nil {
		if err := item.failure.Validate(); err != nil {
			return fmt.Errorf("transcript: Tool failure: %w", err)
		}
	}
	return nil
}

func (item Item) rejectDisallowedPayload() error {
	present := func(value bool, name string) error {
		if value {
			return fmt.Errorf("transcript: %s is not valid for Item kind %s", name, item.kind)
		}
		return nil
	}
	checks := []struct {
		value bool
		name  string
	}{
		{item.kind != UserMessage && item.kind != AgentMessage && len(item.content) != 0, "content"},
		{item.kind != AgentMessage && item.messagePhase != MessagePhaseNone, "AgentMessage phase"},
		{item.kind != Reasoning && item.text != "", "text"},
		{item.kind != Reasoning && item.redacted, "redacted"},
		{item.kind != QuestionItem && item.question != nil, "question"},
		{item.kind != ToolCall && item.tool != nil, "Tool invocation"},
		{item.kind != ToolCall && item.safetyClass != "", "Tool safety class"},
		{item.kind != ToolCall && item.approvalDecision != "", "Tool approval decision"},
		{item.kind != ToolCall && item.failure != nil, "Tool failure"},
		{item.kind != ToolCall && !item.finishedAt.IsZero(), "finish time"},
		{item.kind != ToolCall && item.executionDuration != nil, "execution duration"},
		{item.kind != Compaction && item.summary != "", "summary"},
		{item.kind != Compaction && item.droppedMessages != 0, "dropped messages"},
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
func (invocation ToolInvocation) Validate(pending bool) error {
	if strings.TrimSpace(invocation.Name) == "" || invocation.Name != strings.TrimSpace(invocation.Name) {
		return errors.New("transcript: Tool name is required without surrounding whitespace")
	}
	if pending && (invocation.Result != nil || invocation.Offload != nil) {
		return errors.New("transcript: pending Tool invocation carries a result")
	}
	if invocation.Offload != nil {
		if err := invocation.Offload.Validate(); err != nil {
			return fmt.Errorf("transcript: Tool offload: %w", err)
		}
		if invocation.Result == nil {
			return errors.New("transcript: offloaded Tool result has no preview")
		}
		if _, textual := invocation.Result.String(); !textual {
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

func (item Item) SessionID() string     { return item.identity.SessionID }
func (item Item) RunID() string         { return item.identity.RunID }
func (item Item) ID() string            { return item.identity.ItemID }
func (item Item) Status() ItemStatus    { return item.status }
func (item Item) OccurredAt() time.Time { return item.identity.OccurredAt }
func (item Item) FinishedAt() time.Time { return item.finishedAt }
func (item Item) ExecutionDuration() (time.Duration, bool) {
	if item.executionDuration == nil {
		return 0, false
	}
	return *item.executionDuration, true
}
func (item Item) Kind() ItemKind                      { return item.kind }
func (item Item) MessagePhase() MessagePhase          { return item.messagePhase }
func (item Item) Content() []ContentBlock             { return CloneContent(item.content) }
func (item Item) Text() string                        { return item.text }
func (item Item) Redacted() bool                      { return item.redacted }
func (item Item) SafetyClass() tool.SafetyClass       { return item.safetyClass }
func (item Item) ApprovalDecision() approval.Decision { return item.approvalDecision }
func (item Item) Summary() string                     { return item.summary }
func (item Item) DroppedMessages() int                { return item.droppedMessages }
func (item Item) Question() (Question, bool) {
	if item.question == nil {
		return Question{}, false
	}
	return *cloneQuestion(item.question), true
}
func (item Item) ToolInvocation() (ToolInvocation, bool) {
	if item.tool == nil {
		return ToolInvocation{}, false
	}
	return *cloneToolInvocation(item.tool), true
}

// AnswerQuestion enriches one complete Question prompt with the exact response
// accepted at the resume linearization point. Identity, occurrence, and Item
// status remain unchanged; a second answer is rejected.
func (item Item) AnswerQuestion(answers [][]string) (Item, error) {
	if item.kind != QuestionItem || item.question == nil || item.status != ItemCompleted {
		return Item{}, errors.New("transcript: only a complete Question can be answered")
	}
	if item.question.Answered() {
		return Item{}, errors.New("transcript: Question is already answered")
	}
	item.question = cloneQuestion(item.question)
	item.question.Answers = cloneQuestionAnswers(answers)
	if err := item.Validate(); err != nil {
		return Item{}, err
	}
	return item, nil
}
func (item Item) Failure() (tool.Failure, bool) {
	if item.failure == nil {
		return tool.Failure{}, false
	}
	return *cloneToolFailure(item.failure), true
}
