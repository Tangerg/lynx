package runs

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/lynx/core/chat"
)

var (
	errExecutorContract = errors.New("runs: executor contract violation")
	errReducerInvariant = errors.New("runs: reducer invariant violation")
)

// reduction is one canonical output plus the persisted fact and live nudge that
// arise from the same ExecutionFact decision. The pump commits it before placing
// Event on the journal.
type reduction struct {
	Event  RunEvent
	Commit *EventCommit
	Nudge  *Nudge
}

// reductionBatch is the complete publication unit for one executor event. A
// normal batch commits individual event projections in order. A park batch owns
// one explicit write-set that must commit before any of its events become
// visible; keeping that boundary on the batch avoids encoding it as a boolean
// or a privileged first element in the event slice.
type reductionBatch struct {
	events             []reduction
	parkCommit         *EventCommit
	settledToolCallIDs []string
}

// factReduction is the complete in-memory consequence of one executor fact
// before Run events are projected into their durable publication shape.
type factReduction struct {
	events               []RunEvent
	parkItems            []transcript.Item
	conversationMessages []corechat.Message
	modelInvocations     []ModelInvocationCommit
	toolInvocations      []ToolInvocationCommit
	settledToolCallIDs   []string
	progress             *RunProgressCommit
}

type reducerConfig struct {
	RunID             string
	SegmentID         string
	SessionID         string
	Lineage           run.Lineage
	CWD               string
	ExecutorID        string
	GoalIncarnationID string
	ModelSelection    modelref.Selection
	CreatedAt         time.Time
	UserInput         []transcript.ContentBlock
	// Metrics is what the Run had already consumed before this segment opened —
	// zero for a first segment, the parked Run's accrual for a continuation. Every
	// Run record this reducer commits is the sum of this and the current segment,
	// so a resumed Run reports the Run rather than its latest continuation.
	Metrics run.Metrics
	// Limits is the allowance in force for the whole Run, frozen at admission and
	// carried unchanged through every continuation.
	Limits run.Limits
	// Capabilities is the Run's frozen optional behavior. Every record this reducer
	// commits carries the admission value, including continuation records.
	Capabilities run.Capabilities
	Continuation *treeContinuation
	Now          func() time.Time
	CancelReason func() string
}

// reducer is the per-segment state machine that turns executor events into the
// canonical RunEvent family and EventCommit facts. It owns open item state,
// item identity, resume correlation, terminal synthesis, and error semantics.
type reducer struct {
	cfg     reducerConfig
	resume  *resumeBinding
	itemSeq int
	// step is the latest cumulative accounted model-call count reported by the
	// executor. It uses the same unit as Limits.MaxSteps; tool events never
	// infer it.
	step      int
	toolOrder int
	// usage is the latest authoritative cumulative Run accounting reported by
	// the executor. Nil means this segment has not advanced the committed
	// snapshot in cfg.Metrics.
	usage           *accounting.Usage
	segmentDuration time.Duration
	userInput       []transcript.ContentBlock
	text            *openText
	reasoning       *openText
	modelCalls      map[string]time.Time
	// modelBoundaryClosed fences lossy stream observations that arrive after the
	// authoritative ModelCallCompleted commit. A later ModelCallStarted reopens
	// the observation window for the next provider turn.
	modelBoundaryClosed bool
	// Exactly one side of the final model/process confirmation handshake may be
	// pending. The two facts travel through different executor paths and either
	// can arrive first; only ModelCallCompleted projects transcript content.
	lastModelMessage      *corechat.Message
	earlyAssistantMessage *corechat.Message
	toolCallIDs           map[string]struct{}
	toolPositions         map[toolPosition]string
	tools                 openTools
	drained               []DrainedTool
	errFailure            *run.Failure
	// plan is the last state snapshot this segment published, kept so the segment
	// can fence its final value before finishing. Nil means this segment never
	// changed the projection, and a segment that changed nothing has nothing to
	// fence.
	plan *StateSnapshot
}

type openText struct {
	id        string
	createdAt time.Time
	buf       strings.Builder
}

type openTool struct {
	callID            string
	sourceCallID      string
	arrivalOrder      int
	modelCallSequence uint32
	toolCallIndex     uint32
	id                string
	occurredAt        time.Time
	attemptStartedAt  time.Time
	finishedAt        time.Time
	name              string
	arguments         tool.Arguments
	safetyClass       tool.SafetyClass
	end               *ToolCallFinished
}

type toolPosition struct {
	modelCallSequence uint32
	toolCallIndex     uint32
}

func newReducer(cfg reducerConfig) *reducer {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	cfg.Now = now
	// The reducer outlives the Start request and publishes UserInput through the
	// journal after admission. Own the slice before it becomes persisted/live
	// state so a caller reusing its command buffer cannot rewrite emitted facts.
	cfg.UserInput = slices.Clone(cfg.UserInput)
	var resume *resumeBinding
	if cfg.Continuation != nil {
		resume = resumeBindingFrom(*cfg.Continuation, cfg.RunID)
	}
	return &reducer{
		cfg: cfg, resume: resume, userInput: transcript.CloneContent(cfg.UserInput), step: cfg.Metrics.Steps(),
		modelCalls: make(map[string]time.Time), toolCallIDs: make(map[string]struct{}),
		toolPositions: make(map[toolPosition]string), tools: make(openTools),
	}
}

// clone creates the speculative reducer used by an authoritative fact commit.
// The Run pump swaps it in only after the complete persistence batch succeeds;
// a rejected write therefore cannot consume model/tool state or mint identities
// that the durable projection never observed.
func (r *reducer) clone() *reducer {
	if r == nil {
		return nil
	}
	cloned := *r
	cloned.cfg.UserInput = slices.Clone(r.cfg.UserInput)
	cloned.userInput = transcript.CloneContent(r.userInput)
	cloned.modelCalls = maps.Clone(r.modelCalls)
	cloned.toolCallIDs = maps.Clone(r.toolCallIDs)
	cloned.toolPositions = maps.Clone(r.toolPositions)
	cloned.drained = slices.Clone(r.drained)
	cloned.tools = make(openTools, len(r.tools))
	for callID, current := range r.tools {
		if current == nil {
			cloned.tools[callID] = nil
			continue
		}
		tool := *current
		if current.end != nil {
			end := *current.end
			end.MutatedPaths = slices.Clone(current.end.MutatedPaths)
			if current.end.Result != nil {
				result := *current.end.Result
				end.Result = &result
			}
			if current.end.Offload != nil {
				offload := *current.end.Offload
				end.Offload = &offload
			}
			if current.end.Failure != nil {
				failure := *current.end.Failure
				end.Failure = &failure
			}
			tool.end = &end
		}
		cloned.tools[callID] = &tool
	}
	cloned.text = cloneOpenText(r.text)
	cloned.reasoning = cloneOpenText(r.reasoning)
	cloned.resume = cloneResumeBinding(r.resume)
	if r.plan != nil {
		plan := *r.plan
		plan.Plan = slices.Clone(r.plan.Plan)
		cloned.plan = &plan
	}
	if r.errFailure != nil {
		failure := *r.errFailure
		cloned.errFailure = &failure
	}
	if r.lastModelMessage != nil {
		message := r.lastModelMessage.Clone()
		cloned.lastModelMessage = &message
	}
	if r.earlyAssistantMessage != nil {
		message := r.earlyAssistantMessage.Clone()
		cloned.earlyAssistantMessage = &message
	}
	return &cloned
}

func cloneOpenText(value *openText) *openText {
	if value == nil {
		return nil
	}
	cloned := &openText{id: value.id, createdAt: value.createdAt}
	cloned.buf.WriteString(value.buf.String())
	return cloned
}

func cloneResumeBinding(value *resumeBinding) *resumeBinding {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.callItems = maps.Clone(value.callItems)
	cloned.toolItems = maps.Clone(value.toolItems)
	cloned.byName = maps.Clone(value.byName)
	cloned.drained = slices.Clone(value.drained)
	cloned.committed = maps.Clone(value.committed)
	cloned.consumed = maps.Clone(value.consumed)
	return &cloned
}

func (r *reducer) nextItemID() string {
	r.itemSeq++
	return itemIDPrefix + r.cfg.SegmentID + "_" + strconv.Itoa(r.itemSeq)
}

func userMessageItemID(segmentID string) string { return itemIDPrefix + segmentID + "_u" }

func (r *reducer) open() (reductionBatch, error) {
	if r.resume != nil && r.resume.err != nil {
		return reductionBatch{}, fmt.Errorf("%w: %w", errReducerInvariant, r.resume.err)
	}
	// The opening Run record goes through runRecord like every other one, so a
	// resumed segment announces the Run's accrual and allowance rather than a fresh
	// Run's zeros. Only the creation stamp differs: an opening may have to mint one.
	opening, err := r.runRecord(run.Running)
	if err != nil {
		return reductionBatch{}, err
	}
	out := []RunEvent{SegmentStarted{Run: opening}}
	userMessage, err := r.openUserMessage()
	if err != nil {
		return reductionBatch{}, err
	}
	out = append(out, userMessage...)
	batch, err := r.project(out)
	if err != nil {
		return reductionBatch{}, err
	}
	if r.cfg.Lineage.IsRoot() && len(r.cfg.UserInput) != 0 {
		message, err := MaterializeUserMessage(r.cfg.UserInput)
		if err != nil {
			return reductionBatch{}, fmt.Errorf("%w: opening conversation message: %w", errReducerInvariant, err)
		}
		if err := r.attachConversationMessages(&batch, []corechat.Message{message}); err != nil {
			return reductionBatch{}, err
		}
	}
	return batch, nil
}

func (r *reducer) reduce(ev ExecutionFact) (reductionBatch, error) {
	reduced, err := r.reduceFact(ev)
	if err != nil {
		return reductionBatch{}, err
	}
	return r.projectFact(reduced)
}

func (r *reducer) projectFact(reduced factReduction) (reductionBatch, error) {
	batch, err := r.project(reduced.events)
	if err != nil {
		return reductionBatch{}, err
	}
	batch.settledToolCallIDs = slices.Clone(reduced.settledToolCallIDs)
	if len(reduced.parkItems) != 0 {
		if batch.parkCommit == nil {
			return reductionBatch{}, fmt.Errorf("%w: parked Items have no park boundary", errReducerInvariant)
		}
		batch.parkCommit.Items = append(batch.parkCommit.Items, reduced.parkItems...)
		if err := validateReductionBatch(batch); err != nil {
			return reductionBatch{}, err
		}
	}
	if err := r.attachDurableObservation(
		&batch,
		reduced.conversationMessages,
		reduced.modelInvocations,
		reduced.toolInvocations,
		reduced.progress,
	); err != nil {
		return reductionBatch{}, err
	}
	return batch, nil
}

func (r *reducer) reduceFact(ev ExecutionFact) (factReduction, error) {
	switch e := ev.(type) {
	case MessageDelta:
		// Reasoning and message are two concurrent projections of one model
		// response, not mutually exclusive stream modes. Keep each Item open until
		// ModelCallCompleted supplies the authoritative full message; completing one
		// merely because the other emitted would make that final message duplicate it.
		if r.modelBoundaryClosed {
			return factReduction{}, nil
		}
		appended, err := r.appendText(e.Text)
		if err != nil {
			return factReduction{}, fmt.Errorf("%w: append text: %w", errReducerInvariant, err)
		}
		return factReduction{events: appended}, nil
	case ReasoningDelta:
		if r.modelBoundaryClosed {
			return factReduction{}, nil
		}
		appended, err := r.appendReasoning(e.Text)
		if err != nil {
			return factReduction{}, fmt.Errorf("%w: append reasoning: %w", errReducerInvariant, err)
		}
		return factReduction{events: appended}, nil
	case AssistantMessageCompleted:
		return r.reduceAssistantMessage(e)
	case ModelCallStarted:
		return r.startModelCall(e)
	case ModelCallCompleted:
		return r.completeModelCall(e)
	case ModelCallFailed:
		return r.failModelCall(e)
	case ToolCallStarted:
		return r.startToolCall(e)
	case ToolCallFinished:
		return r.finishToolCall(e)
	case UsageReported:
		events, err := r.usageProgress(e)
		if err != nil {
			return factReduction{}, fmt.Errorf("%w: usage report: %w", errExecutorContract, err)
		}
		return factReduction{events: events}, nil
	case SteerMessagesApplied:
		events, err := r.steerMessagesApplied(e)
		if err != nil {
			return factReduction{}, fmt.Errorf("%w: applied steers: %w", errExecutorContract, err)
		}
		var messages []corechat.Message
		if r.cfg.Lineage.IsRoot() {
			messages = make([]corechat.Message, len(e.Messages))
			for index, content := range e.Messages {
				message, err := MaterializeUserMessage(content)
				if err != nil {
					return factReduction{}, fmt.Errorf("%w: applied steer message[%d]: %w", errExecutorContract, index, err)
				}
				messages[index] = message
			}
		}
		return factReduction{events: events, conversationMessages: messages}, nil
	case PlanUpdated:
		return factReduction{events: r.planSnapshot(e)}, nil
	case CompactionBoundary:
		events, err := r.compaction(e)
		if err != nil {
			return factReduction{}, fmt.Errorf("%w: compaction: %w", errReducerInvariant, err)
		}
		return factReduction{events: events}, nil
	case SegmentInterrupted:
		interrupted, err := r.interrupt(e)
		if err != nil {
			return factReduction{}, fmt.Errorf("%w: interrupt: %w", errExecutorContract, err)
		}
		return interrupted, nil
	case SegmentEnded:
		return r.endSegment(e)
	default:
		return factReduction{}, fmt.Errorf("%w: unhandled event %T", errExecutorContract, ev)
	}
}

func (r *reducer) reduceAssistantMessage(completed AssistantMessageCompleted) (factReduction, error) {
	if r.lastModelMessage != nil {
		if !reflect.DeepEqual(*r.lastModelMessage, completed.Message) {
			return factReduction{}, fmt.Errorf(
				"%w: executor final assistant message differs from the last committed model response",
				errExecutorContract,
			)
		}
		r.lastModelMessage = nil
		return factReduction{}, nil
	}
	if r.earlyAssistantMessage != nil {
		return factReduction{}, fmt.Errorf(
			"%w: executor completed the assistant message more than once",
			errExecutorContract,
		)
	}
	message := completed.Message.Clone()
	r.earlyAssistantMessage = &message
	return factReduction{}, nil
}

func (r *reducer) startModelCall(started ModelCallStarted) (factReduction, error) {
	if strings.TrimSpace(started.CallID) == "" || started.CallID != strings.TrimSpace(started.CallID) {
		return factReduction{}, fmt.Errorf("%w: model call start has an invalid id", errExecutorContract)
	}
	if _, duplicate := r.modelCalls[started.CallID]; duplicate {
		return factReduction{}, fmt.Errorf("%w: model call %q started more than once", errExecutorContract, started.CallID)
	}
	startedAt := r.now()
	r.modelCalls[started.CallID] = startedAt
	r.modelBoundaryClosed = false
	return factReduction{
		events: []RunEvent{SegmentProgressed{Progress: RunProgress{Activity: "Calling model"}}},
		modelInvocations: []ModelInvocationCommit{{
			CallID: started.CallID, SegmentID: r.cfg.SegmentID,
			State: ModelInvocationStarted, StartedAt: startedAt,
		}},
	}, nil
}

func (r *reducer) completeModelCall(completed ModelCallCompleted) (factReduction, error) {
	if strings.TrimSpace(completed.CallID) == "" || completed.CallID != strings.TrimSpace(completed.CallID) {
		return factReduction{}, fmt.Errorf("%w: model call completion has an invalid id", errExecutorContract)
	}
	startedAt, started := r.modelCalls[completed.CallID]
	if !started {
		return factReduction{}, fmt.Errorf("%w: model call %q completed without a start", errExecutorContract, completed.CallID)
	}
	finishedAt := r.now()
	if finishedAt.Before(startedAt) {
		return factReduction{}, fmt.Errorf(
			"%w: model call %q completion precedes its start",
			errExecutorContract,
			completed.CallID,
		)
	}
	delete(r.modelCalls, completed.CallID)
	r.modelBoundaryClosed = true
	if r.earlyAssistantMessage != nil && !reflect.DeepEqual(*r.earlyAssistantMessage, completed.Message) {
		return factReduction{}, fmt.Errorf(
			"%w: executor final assistant message differs from the committed model response",
			errExecutorContract,
		)
	}
	events, err := r.completeModelMessage(completed.Message)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: model call completion: %w", errExecutorContract, err)
	}
	if r.earlyAssistantMessage != nil {
		r.earlyAssistantMessage = nil
		r.lastModelMessage = nil
	} else if !messageRequestsToolCalls(completed.Message) {
		message := completed.Message.Clone()
		r.lastModelMessage = &message
	} else {
		r.lastModelMessage = nil
	}
	progressEvents, err := r.usageProgress(UsageReported{
		TokenUsage: completed.TokenUsage, ByModel: completed.ByModel, CostUSD: completed.CostUSD,
		Steps: completed.Steps, ContextTokens: completed.ContextTokens,
	})
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: model call usage: %w", errExecutorContract, err)
	}
	metrics, err := r.metrics()
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: model call metrics: %w", errExecutorContract, err)
	}
	return factReduction{
		events:               append(events, progressEvents...),
		conversationMessages: r.rootConversationMessages(completed.Message),
		modelInvocations: []ModelInvocationCommit{{
			CallID: completed.CallID, SegmentID: r.cfg.SegmentID,
			State: ModelInvocationCompleted, StartedAt: startedAt, FinishedAt: finishedAt,
		}},
		progress: &RunProgressCommit{
			SegmentID: r.cfg.SegmentID, Metrics: metrics, UpdatedAt: finishedAt,
		},
	}, nil
}

func (r *reducer) failModelCall(failed ModelCallFailed) (factReduction, error) {
	if strings.TrimSpace(failed.CallID) == "" || failed.CallID != strings.TrimSpace(failed.CallID) {
		return factReduction{}, fmt.Errorf("%w: model call failure has an invalid id", errExecutorContract)
	}
	startedAt, started := r.modelCalls[failed.CallID]
	if !started {
		return factReduction{}, fmt.Errorf("%w: model call %q failed without a start", errExecutorContract, failed.CallID)
	}
	finishedAt := r.now()
	if finishedAt.Before(startedAt) {
		return factReduction{}, fmt.Errorf(
			"%w: model call %q failure precedes its start",
			errExecutorContract,
			failed.CallID,
		)
	}
	delete(r.modelCalls, failed.CallID)
	return factReduction{
		events: []RunEvent{SegmentProgressed{Progress: RunProgress{Activity: "Model call failed"}}},
		modelInvocations: []ModelInvocationCommit{{
			CallID: failed.CallID, SegmentID: r.cfg.SegmentID,
			State: ModelInvocationFailed, StartedAt: startedAt, FinishedAt: finishedAt,
		}},
	}, nil
}

func (r *reducer) startToolCall(started ToolCallStarted) (factReduction, error) {
	events, err := r.toolStart(started)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: tool call start: %w", errExecutorContract, err)
	}
	reduced := factReduction{events: events}
	if ref := r.tools[started.CallID]; ref != nil && ref.modelCallSequence > 0 {
		reduced.toolInvocations = []ToolInvocationCommit{{
			CallID: ref.callID, ItemID: ref.id, SegmentID: r.cfg.SegmentID,
			State: ToolInvocationStarted, StartedAt: ref.attemptStartedAt,
		}}
	}
	return reduced, nil
}

func (r *reducer) finishToolCall(finished ToolCallFinished) (factReduction, error) {
	events, invocations, messages, err := r.toolEnd(finished)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: tool call end: %w", errExecutorContract, err)
	}
	settledCallIDs := make([]string, len(invocations))
	for index, invocation := range invocations {
		settledCallIDs[index] = invocation.CallID
	}
	return factReduction{
		events: events, conversationMessages: messages,
		toolInvocations: invocations, settledToolCallIDs: settledCallIDs,
	}, nil
}

func (r *reducer) endSegment(ended SegmentEnded) (factReduction, error) {
	if len(r.modelCalls) > 0 && ended.Reason != run.OutcomeLost {
		return factReduction{}, fmt.Errorf(
			"%w: segment ended with %d unsettled model calls",
			errExecutorContract,
			len(r.modelCalls),
		)
	}
	modelInvocations, err := r.closeLostModelCalls(ended.Reason)
	if err != nil {
		return factReduction{}, err
	}
	openTools := r.tools.ordered()
	events, err := r.segmentEnd(ended)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: segment end: %w", errExecutorContract, err)
	}
	return factReduction{
		events:           events,
		modelInvocations: modelInvocations,
		toolInvocations:  closedToolInvocationCommits(r.cfg.SegmentID, openTools),
	}, nil
}

func (r *reducer) closeLostModelCalls(outcome run.Outcome) ([]ModelInvocationCommit, error) {
	if outcome != run.OutcomeLost {
		return nil, nil
	}
	finishedAt := r.now()
	callIDs := slices.Sorted(maps.Keys(r.modelCalls))
	invocations := make([]ModelInvocationCommit, 0, len(callIDs))
	for _, callID := range callIDs {
		startedAt := r.modelCalls[callID]
		if finishedAt.Before(startedAt) {
			return nil, fmt.Errorf("%w: model call %q loss precedes its start", errExecutorContract, callID)
		}
		invocations = append(invocations, ModelInvocationCommit{
			CallID: callID, SegmentID: r.cfg.SegmentID,
			State: ModelInvocationUnknown, StartedAt: startedAt, FinishedAt: finishedAt,
		})
	}
	clear(r.modelCalls)
	return invocations, nil
}

func (r *reducer) attachDurableObservation(
	batch *reductionBatch,
	conversationMessages []corechat.Message,
	modelInvocations []ModelInvocationCommit,
	toolInvocations []ToolInvocationCommit,
	progress *RunProgressCommit,
) error {
	if len(conversationMessages) == 0 && len(modelInvocations) == 0 && len(toolInvocations) == 0 && progress == nil {
		return nil
	}
	if batch == nil || len(batch.events) == 0 {
		return fmt.Errorf("%w: durable observation has no ordinary reduction", errReducerInvariant)
	}
	var commit *EventCommit
	if batch.parkCommit != nil {
		commit = batch.parkCommit
	} else {
		last := &batch.events[len(batch.events)-1]
		if last.Commit == nil {
			last.Commit = &EventCommit{RunID: r.cfg.RunID, SessionID: r.cfg.SessionID}
		}
		commit = last.Commit
	}
	commit.ModelInvocations = append(commit.ModelInvocations, modelInvocations...)
	commit.ToolInvocations = append(commit.ToolInvocations, toolInvocations...)
	commit.ConversationMessages = appendClonedMessages(commit.ConversationMessages, conversationMessages...)
	if progress != nil {
		cloned := *progress
		commit.Progress = &cloned
	}
	return validateReductionBatch(*batch)
}

func (r *reducer) attachConversationMessages(batch *reductionBatch, messages []corechat.Message) error {
	if len(messages) == 0 {
		return nil
	}
	if batch == nil || len(batch.events) == 0 || batch.parkCommit != nil {
		return fmt.Errorf("%w: conversation projection has no ordinary reduction", errReducerInvariant)
	}
	last := &batch.events[len(batch.events)-1]
	if last.Commit == nil {
		last.Commit = &EventCommit{RunID: r.cfg.RunID, SessionID: r.cfg.SessionID}
	}
	last.Commit.ConversationMessages = appendClonedMessages(last.Commit.ConversationMessages, messages...)
	return validateReductionBatch(*batch)
}

func (r *reducer) rootConversationMessages(messages ...corechat.Message) []corechat.Message {
	if !r.cfg.Lineage.IsRoot() {
		return nil
	}
	return appendClonedMessages(nil, messages...)
}

func appendClonedMessages(dst []corechat.Message, messages ...corechat.Message) []corechat.Message {
	for _, message := range messages {
		dst = append(dst, message.Clone())
	}
	return dst
}

func closedToolInvocationCommits(segmentID string, tools []*openTool) []ToolInvocationCommit {
	commits := make([]ToolInvocationCommit, 0, len(tools))
	for _, ref := range tools {
		if ref == nil || ref.modelCallSequence == 0 || ref.finishedAt.IsZero() {
			continue
		}
		state := ToolInvocationIncomplete
		if ref.end != nil {
			state = ToolInvocationCompleted
		}
		commits = append(commits, ToolInvocationCommit{
			CallID: ref.callID, ItemID: ref.id, SegmentID: segmentID,
			State: state, StartedAt: ref.attemptStartedAt, FinishedAt: ref.finishedAt,
		})
	}
	return commits
}

func (r *reducer) synthesizeTerminal() (reductionBatch, error) {
	out, err := r.closeStreaming()
	if err != nil {
		return reductionBatch{}, fmt.Errorf("%w: close streaming: %w", errReducerInvariant, err)
	}
	resumedTools, err := r.abandonUnconsumedResumeTools()
	if err != nil {
		return reductionBatch{}, fmt.Errorf("%w: abandon resumed tools: %w", errReducerInvariant, err)
	}
	out = append(out, resumedTools...)
	openTools := r.tools.ordered()
	drained, err := r.drainTools()
	if err != nil {
		return reductionBatch{}, fmt.Errorf("%w: drain tools: %w", errReducerInvariant, err)
	}
	out = append(out, drained...)
	// No SegmentEnded arrived, so nothing fresh was reported: the Segment's accrual
	// stands as last reported and is committed as-is.
	var failure *run.Failure
	var modelInvocations []ModelInvocationCommit
	outcome := run.OutcomeCanceled
	if len(r.modelCalls) > 0 {
		outcome = run.OutcomeLost
		failure = &run.Failure{
			Kind:   run.FailureLost,
			Detail: "a model invocation ended without a provable durable result",
		}
		finishedAt := r.now()
		callIDs := slices.Sorted(maps.Keys(r.modelCalls))
		modelInvocations = make([]ModelInvocationCommit, 0, len(callIDs))
		for _, callID := range callIDs {
			startedAt := r.modelCalls[callID]
			if finishedAt.Before(startedAt) {
				return reductionBatch{}, fmt.Errorf("%w: model call %q loss precedes its start", errReducerInvariant, callID)
			}
			modelInvocations = append(modelInvocations, ModelInvocationCommit{
				CallID: callID, SegmentID: r.cfg.SegmentID,
				State: ModelInvocationUnknown, StartedAt: startedAt, FinishedAt: finishedAt,
			})
		}
		clear(r.modelCalls)
	} else if r.errFailure != nil {
		outcome = run.OutcomeFailed
		failure = r.errFailure
	}
	detail := ""
	if outcome == run.OutcomeCanceled && r.cfg.CancelReason != nil {
		detail = r.cfg.CancelReason()
	}
	terminal, err := r.finishedRun(outcome, failure, detail)
	if err != nil {
		return reductionBatch{}, fmt.Errorf("%w: synthesize terminal: %w", errReducerInvariant, err)
	}
	out = append(out, terminal)
	batch, err := r.project(out)
	if err != nil {
		return reductionBatch{}, err
	}
	if err := r.attachDurableObservation(
		&batch,
		nil,
		modelInvocations,
		closedToolInvocationCommits(r.cfg.SegmentID, openTools),
		nil,
	); err != nil {
		return reductionBatch{}, err
	}
	return batch, nil
}

// abandonUnconsumedResumeTools closes logical Tool Items that were carried
// across a human boundary but whose executor attempt never restarted in this
// Segment. Without this step an activation failure or a cancel-before-activate
// can terminalize the Run while leaving its preexisting Tool Item running.
func (r *reducer) abandonUnconsumedResumeTools() ([]RunEvent, error) {
	if r.resume == nil {
		return nil, nil
	}
	remaining := r.resume.remainingDrainedTools()
	events := make([]RunEvent, 0, len(remaining))
	for _, drained := range remaining {
		arguments, err := parseToolArguments(drained.Arguments)
		if err != nil {
			return nil, fmt.Errorf("tool %q arguments: %w", drained.Name, err)
		}
		ref := &openTool{
			callID:       drained.CallID,
			sourceCallID: drained.SourceCallID,
			id:           drained.ItemID,
			occurredAt:   drained.ItemOccurredAt,
			name:         drained.Name,
			arguments:    arguments,
		}
		completed, err := r.abandonUnstartedToolItem(ref)
		if err != nil {
			return nil, err
		}
		events = append(events, completed)
		r.resume.consumeToolItem(drained.ItemID)
	}
	return events, nil
}

// abort marks the Segment as failed so terminal synthesis produces an error
// outcome. It takes no cause: an internal failure exposes only its stable problem
// kind to observers.
// That makes the caller's span the only place the cause survives — a rejected
// terminal commit or a contract-violating executor event is otherwise invisible
// — so every caller records it there before calling this.
func (r *reducer) abort() {
	r.errFailure = &run.Failure{Kind: run.FailureInternal}
}

func (r *reducer) project(events []RunEvent) (reductionBatch, error) {
	events = r.fenceFinalState(events)
	reductions := make([]reduction, 0, len(events))
	for _, event := range events {
		reduced, err := r.projectOne(event)
		if err != nil {
			return reductionBatch{}, err
		}
		reductions = append(reductions, reduced)
	}

	// A park is one persistence boundary: any drained/closed items, its running
	// approval/question items, open interrupt record, waiting transcript Run,
	// and admission transition must commit together before ANY event in this
	// batch is published. Build an explicit batch-owned write-set instead of
	// moving it onto a privileged event position.
	parkBoundary, err := parkBoundaryIndex(reductions)
	if err != nil {
		return reductionBatch{}, err
	}
	batch := reductionBatch{events: reductions}
	if parkBoundary >= 0 {
		batch, err = parkReductionBatch(reductions, parkBoundary)
		if err != nil {
			return reductionBatch{}, err
		}
	}
	if err := validateReductionBatch(batch); err != nil {
		return reductionBatch{}, err
	}
	return batch, nil
}

func parkBoundaryIndex(reductions []reduction) (int, error) {
	parkBoundary := -1
	for index := range reductions {
		commit := reductions[index].Commit
		if commit == nil || commit.State != StateSuspend {
			continue
		}
		if parkBoundary >= 0 {
			return -1, fmt.Errorf("%w: reduction batch has multiple park boundaries", errReducerInvariant)
		}
		parkBoundary = index
	}
	return parkBoundary, nil
}

func parkReductionBatch(reductions []reduction, parkBoundary int) (reductionBatch, error) {
	parkCommit := reductions[parkBoundary].Commit
	if parkCommit == nil {
		return reductionBatch{}, fmt.Errorf("%w: park boundary has no projection commit", errReducerInvariant)
	}
	for index, reduced := range reductions {
		if index != parkBoundary && reduced.Commit != nil {
			if reduced.Commit.Run != nil || reduced.Commit.State != StateUnchanged {
				return reductionBatch{}, fmt.Errorf("%w: park batch contains another lifecycle transition", errReducerInvariant)
			}
		}
		reductions[index].Commit = nil
	}
	return reductionBatch{events: reductions, parkCommit: parkCommit}, nil
}

// fenceFinalState republishes the segment's last state snapshot immediately before
// the segment finishes, for every key the segment changed.
//
// Without it, a client only holds the state if it received the change event itself.
// A subscriber that attached later — or replayed from a cursor past that event —
// reaches segment.finished having never seen a snapshot, and renders a stale panel
// until something makes it refetch. The fence makes the guarantee positional:
// whoever receives the finish has received the final value, because it is the
// replayable event immediately before it.
//
// The repeat is the point, not waste: a latest-value projection carries its own
// revision, so folding it twice is folding it once.
//
// It belongs to the batch rather than to either finish path: a park and a terminal
// are two reasons for one boundary, and a rule stated in both places is a rule that
// drifts in one of them.
func (r *reducer) fenceFinalState(events []RunEvent) []RunEvent {
	if r.plan == nil {
		return events
	}
	for i, event := range events {
		if _, finishing := event.(SegmentFinished); !finishing {
			continue
		}
		fence := *r.plan
		// One fence per segment: a resumed segment fences again only if it changes
		// the projection again.
		r.plan = nil
		return slices.Insert(events, i, RunEvent(fence))
	}
	return events
}

func (r *reducer) projectOne(event RunEvent) (reduction, error) {
	commit := EventCommit{RunID: r.cfg.RunID, SessionID: r.cfg.SessionID}
	var nudge *Nudge
	switch e := event.(type) {
	case ItemCompleted:
		commit.Items = []transcript.Item{e.Item}
		if len(e.mutatedPaths) > 0 {
			nudge = &Nudge{CWD: r.cfg.CWD, Paths: slices.Clone(e.mutatedPaths)}
		}
	case SegmentFinished:
		commit.Run = &e.Run
		if e.Run.State() == run.Waiting {
			commit.State = StateSuspend
			return reduction{Event: event, Commit: &commit}, nil
		}
		commit.State = StateTerminalize
		if outcome, terminal := e.Run.Outcome(); terminal {
			commit.Outcome = outcome
			commit.GoalRun = r.goalTurn(e.Run)
		}
	case ItemStarted:
		if err := e.Item.validate(); err != nil {
			return reduction{}, fmt.Errorf("%w: Item start: %v", errReducerInvariant, err)
		}
	case SegmentStarted, SegmentProgressed, ItemChanged, StateSnapshot:
		// These events have no standalone EventCommit. SegmentStarted carries a Run
		// for the stream, but the Run's durable opening IS its admission (or its
		// resume) — recording it a second time here would be a second writer of
		// facts admission already owns. ItemStarted projections remain provisional;
		// interrupt starts are folded into the atomic park write-set by project.
	default:
		return reduction{}, fmt.Errorf("%w: unhandled run event %T", errReducerInvariant, event)
	}
	var eventCommit *EventCommit
	if !commit.isEmpty() {
		eventCommit = &commit
	}
	return reduction{Event: event, Commit: eventCommit, Nudge: nudge}, nil
}

func (r *reducer) goalTurn(run run.Run) *goal.RunRecord {
	outcome, terminal := run.Outcome()
	if r.cfg.GoalIncarnationID == "" || !terminal {
		return nil
	}
	record := &goal.RunRecord{
		SessionID:     r.cfg.SessionID,
		IncarnationID: r.cfg.GoalIncarnationID,
		RunID:         r.cfg.RunID,
		Outcome:       outcome,
		CompletedAt:   run.FinishedAt(),
	}
	if record.CompletedAt.IsZero() {
		record.CompletedAt = r.now()
	}
	record.Steps = run.Metrics().Steps()
	if usage, reported := run.Metrics().Usage(); reported && usage.Total.CostUSD != nil {
		record.CostUSD = *usage.Total.CostUSD
	}
	return record
}

// validateReductionBatch checks the pump-facing shape before any commit or
// publication occurs. The reducer normally constructs this shape itself; the
// second check keeps future projection changes from creating partial persistence
// boundaries.
func validateReductionBatch(batch reductionBatch) error {
	if len(batch.events) == 0 {
		if batch.parkCommit != nil {
			return fmt.Errorf("%w: empty batch has a park commit", errReducerInvariant)
		}
		return nil
	}

	terminalAt, err := validateReductionEvents(batch.events)
	if err != nil {
		return err
	}
	if batch.parkCommit != nil {
		return validateParkReductionBatch(batch, terminalAt)
	}
	if terminalAt < 0 {
		return nil
	}
	return validateTerminalReduction(batch.events[terminalAt])
}

// lifecycleReductions counts the events that move the segment's lifecycle: its
// opening and its ending.
func lifecycleReductions(reductions []reduction) int {
	count := 0
	for _, reduced := range reductions {
		switch reduced.Event.(type) {
		case SegmentStarted, SegmentFinished:
			count++
		}
	}
	return count
}

func validateReductionEvents(reductions []reduction) (terminalAt int, err error) {
	terminalAt = -1
	for i, reduced := range reductions {
		if reduced.Event == nil {
			return -1, fmt.Errorf("%w: reduction[%d] has no event", errReducerInvariant, i)
		}
		// One batch moves the segment's lifecycle at most once, and an opening only
		// ever starts one. Both are checked on the events rather than on their
		// projection commits because an opening produces none — a Run's persisted
		// opening is its admission — so a stray one would otherwise pass unseen.
		if _, opening := reduced.Event.(SegmentStarted); opening {
			if i != 0 {
				return -1, fmt.Errorf("%w: reduction[%d] opens a segment mid-batch", errReducerInvariant, i)
			}
			if lifecycleReductions(reductions) > 1 {
				return -1, fmt.Errorf("%w: reduction batch both opens and ends a segment", errReducerInvariant)
			}
		}
		if reduced.Event.Terminal() {
			terminalAt = i
			if i != len(reductions)-1 {
				return -1, fmt.Errorf("%w: terminal reduction[%d] is not last", errReducerInvariant, i)
			}
		}
		if reduced.Commit != nil {
			switch reduced.Commit.State {
			case StateUnchanged:
			case StateSuspend:
				return -1, fmt.Errorf("%w: reduction[%d] carries a park commit", errReducerInvariant, i)
			case StateTerminalize:
				if !reduced.Event.Terminal() {
					return -1, fmt.Errorf("%w: terminal commit at reduction[%d] has no terminal event", errReducerInvariant, i)
				}
			default:
				return -1, fmt.Errorf("%w: reduction[%d] has unknown state change %d", errReducerInvariant, i, reduced.Commit.State)
			}
		}
	}
	return terminalAt, nil
}

func validateParkReductionBatch(batch reductionBatch, terminalAt int) error {
	for i, reduced := range batch.events {
		if reduced.Commit != nil {
			return fmt.Errorf("%w: park batch event[%d] repeats a projection commit", errReducerInvariant, i)
		}
	}
	commit := batch.parkCommit
	switch {
	case commit == nil:
		return fmt.Errorf("%w: park batch has no projection commit", errReducerInvariant)
	case commit.State != StateSuspend:
		return fmt.Errorf("%w: park batch commit does not suspend the run", errReducerInvariant)
	case commit.Run == nil || commit.Run.State() != run.Waiting:
		return fmt.Errorf("%w: park batch commit has no waiting Run", errReducerInvariant)
	case terminalAt != len(batch.events)-1:
		return fmt.Errorf("%w: park batch has no terminal boundary event", errReducerInvariant)
	}
	return nil
}

func validateTerminalReduction(reduced reduction) error {
	commit := reduced.Commit
	switch {
	case commit == nil:
		return fmt.Errorf("%w: terminal event has no projection commit", errReducerInvariant)
	case commit.State != StateTerminalize:
		return fmt.Errorf("%w: terminal event commit does not terminalize the run", errReducerInvariant)
	case commit.Run == nil || !commit.Run.State().IsTerminal():
		return fmt.Errorf("%w: terminal event commit has no terminal run", errReducerInvariant)
	case commit.GoalRun != nil && (commit.GoalRun.RunID != commit.RunID || commit.GoalRun.SessionID != commit.SessionID || commit.GoalRun.Outcome != commit.Outcome):
		return fmt.Errorf("%w: terminal event commit has an inconsistent Goal Run", errReducerInvariant)
	}
	if err := commit.Validate(); err != nil {
		return fmt.Errorf("%w: %w", errReducerInvariant, err)
	}
	wantState, ok := run.Running.Terminate(commit.Outcome)
	committedOutcome, terminal := commit.Run.Outcome()
	if !terminal || committedOutcome != commit.Outcome {
		return fmt.Errorf("%w: terminal event commit has an inconsistent outcome", errReducerInvariant)
	}
	if !ok || commit.Run.State() != wantState {
		return fmt.Errorf("%w: terminal event commit has an invalid lifecycle transition", errReducerInvariant)
	}
	return nil
}

func (r *reducer) now() time.Time {
	now := r.cfg.Now().UTC()
	createdAt := r.cfg.CreatedAt.UTC()
	if !createdAt.IsZero() && now.Before(createdAt) {
		return createdAt
	}
	return now
}
