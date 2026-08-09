package runs

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

func itemPair(build func(transcript.ItemStatus) transcript.Item) []RunEvent {
	return []RunEvent{
		ItemStarted{Item: build(transcript.ItemRunning)},
		ItemCompleted{Item: build(transcript.ItemCompleted)},
	}
}

func (r *reducer) appendText(text string) []RunEvent {
	var out []RunEvent
	if r.text == nil {
		r.text = &openText{id: r.nextItemID(), createdAt: r.now()}
		out = append(out, ItemStarted{Item: transcript.Item{
			ID: r.text.id, RunID: r.cfg.RunID, Status: transcript.ItemRunning,
			Kind: transcript.AgentMessage, OccurredAt: r.text.createdAt,
		}})
	}
	r.text.buf.WriteString(text)
	index := 0
	return append(out, ItemChanged{
		ItemID: r.text.id,
		Delta:  ItemDelta{Kind: ContentDelta, Index: &index, Text: text},
	})
}

func (r *reducer) appendReasoning(text string) []RunEvent {
	var out []RunEvent
	if r.reasoning == nil {
		r.reasoning = &openText{id: r.nextItemID(), createdAt: r.now()}
		out = append(out, ItemStarted{Item: transcript.Item{
			ID: r.reasoning.id, RunID: r.cfg.RunID, Status: transcript.ItemRunning,
			Kind: transcript.Reasoning, OccurredAt: r.reasoning.createdAt,
		}})
	}
	r.reasoning.buf.WriteString(text)
	return append(out, ItemChanged{
		ItemID: r.reasoning.id,
		Delta:  ItemDelta{Kind: ReasoningDeltaKind, Text: text},
	})
}

func (r *reducer) closeText() []RunEvent {
	if r.text == nil {
		return nil
	}
	event := ItemCompleted{Item: transcript.Item{
		ID: r.text.id, RunID: r.cfg.RunID, Status: transcript.ItemCompleted,
		Kind: transcript.AgentMessage, OccurredAt: r.text.createdAt,
		Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: r.text.buf.String()}},
	}}
	r.text = nil
	return []RunEvent{event}
}

func (r *reducer) closeReasoning() []RunEvent {
	if r.reasoning == nil {
		return nil
	}
	event := ItemCompleted{Item: transcript.Item{
		ID: r.reasoning.id, RunID: r.cfg.RunID, Status: transcript.ItemCompleted,
		Kind: transcript.Reasoning, OccurredAt: r.reasoning.createdAt,
		Text: r.reasoning.buf.String(),
	}}
	r.reasoning = nil
	return []RunEvent{event}
}

func (r *reducer) closeStreaming() []RunEvent {
	return append(r.closeReasoning(), r.closeText()...)
}

func (r *reducer) completeAssistantMessage(message corechat.Message) ([]RunEvent, error) {
	if message.Role != corechat.RoleAssistant {
		return nil, fmt.Errorf("completed message role is %q, want %q", message.Role, corechat.RoleAssistant)
	}
	if err := message.Validate(); err != nil {
		return nil, err
	}

	var reasoning strings.Builder
	content := make([]transcript.ContentBlock, 0, len(message.Parts))
	for index, part := range message.Parts {
		switch part.Kind {
		case corechat.PartText:
			content = append(content, transcript.ContentBlock{Kind: transcript.TextContent, Text: part.Text})
		case corechat.PartReasoning:
			reasoning.WriteString(part.Text)
		case corechat.PartMedia:
			block, err := assistantMediaBlock(part.Media)
			if err != nil {
				return nil, fmt.Errorf("part[%d]: %w", index, err)
			}
			content = append(content, block)
		case corechat.PartToolCall:
			return nil, fmt.Errorf("part[%d]: completed assistant message still contains a tool call", index)
		default:
			return nil, fmt.Errorf("part[%d]: unsupported assistant part kind %q", index, part.Kind)
		}
	}

	out := r.completeReasoning(reasoning.String())
	out = append(out, r.completeMessageContent(content)...)
	return out, nil
}

func (r *reducer) completeModelMessage(message corechat.Message) ([]RunEvent, error) {
	if message.Role != corechat.RoleAssistant {
		return nil, fmt.Errorf("completed model message role is %q, want %q", message.Role, corechat.RoleAssistant)
	}
	if err := message.Validate(); err != nil {
		return nil, err
	}
	semantic := corechat.Message{Role: message.Role}
	for _, part := range message.Parts {
		if part.Kind != corechat.PartToolCall {
			semantic.Parts = append(semantic.Parts, part.Clone())
		}
	}
	if len(semantic.Parts) == 0 {
		return r.closeStreaming(), nil
	}
	return r.completeAssistantMessage(semantic)
}

func assistantMediaBlock(value *media.Media) (transcript.ContentBlock, error) {
	if value == nil {
		return transcript.ContentBlock{}, errors.New("assistant media is nil")
	}
	if !strings.HasPrefix(value.MIME, "image/") {
		return transcript.ContentBlock{}, fmt.Errorf("assistant media type %q is not supported by Transcript", value.MIME)
	}
	data, err := value.Bytes()
	if err != nil {
		return transcript.ContentBlock{}, fmt.Errorf("assistant image must use an inline byte source: %w", err)
	}
	return transcript.ContentBlock{Kind: transcript.ImageContent, MediaType: value.MIME, Bytes: data}, nil
}

func (r *reducer) completeReasoning(text string) []RunEvent {
	if text == "" {
		return r.closeReasoning()
	}
	createdAt := r.now()
	id := r.nextItemID()
	started := true
	if r.reasoning != nil {
		createdAt = r.reasoning.createdAt
		id = r.reasoning.id
		started = false
	}
	r.reasoning = nil
	out := make([]RunEvent, 0, 2)
	if started {
		out = append(out, ItemStarted{Item: transcript.Item{
			ID: id, RunID: r.cfg.RunID, Status: transcript.ItemRunning,
			Kind: transcript.Reasoning, OccurredAt: createdAt,
		}})
	}
	out = append(out, ItemCompleted{Item: transcript.Item{
		ID: id, RunID: r.cfg.RunID, Status: transcript.ItemCompleted,
		Kind: transcript.Reasoning, OccurredAt: createdAt, Text: text,
	}})
	return out
}

func (r *reducer) completeMessageContent(content []transcript.ContentBlock) []RunEvent {
	if len(content) == 0 {
		return r.closeText()
	}
	createdAt := r.now()
	id := r.nextItemID()
	started := true
	if r.text != nil {
		createdAt = r.text.createdAt
		id = r.text.id
		started = false
	}
	r.text = nil
	out := make([]RunEvent, 0, 2)
	if started {
		out = append(out, ItemStarted{Item: transcript.Item{
			ID: id, RunID: r.cfg.RunID, Status: transcript.ItemRunning,
			Kind: transcript.AgentMessage, OccurredAt: createdAt,
		}})
	}
	out = append(out, ItemCompleted{Item: transcript.Item{
		ID: id, RunID: r.cfg.RunID, Status: transcript.ItemCompleted,
		Kind: transcript.AgentMessage, OccurredAt: createdAt,
		Content: transcript.CloneContent(content),
	}})
	return out
}

func (r *reducer) toolStart(e ToolCallStarted) ([]RunEvent, error) {
	if strings.TrimSpace(e.CallID) == "" || e.CallID != strings.TrimSpace(e.CallID) {
		return nil, errors.New("tool call id is required")
	}
	if strings.TrimSpace(e.ToolName) == "" || e.ToolName != strings.TrimSpace(e.ToolName) {
		return nil, errors.New("tool name is required")
	}
	if e.SourceCallID != strings.TrimSpace(e.SourceCallID) {
		return nil, errors.New("tool source call id has surrounding whitespace")
	}
	if e.Activity != strings.TrimSpace(e.Activity) {
		return nil, errors.New("tool activity has surrounding whitespace")
	}
	if e.ModelCallSequence == 0 && e.ToolCallIndex != 0 {
		return nil, errors.New("tool call index requires a model call sequence")
	}
	if e.ModelCallSequence > 0 && e.SourceCallID == "" {
		return nil, errors.New("model-attributed tool call requires a source call id")
	}
	if _, duplicate := r.toolCallIDs[e.CallID]; duplicate {
		return nil, fmt.Errorf("tool call %q started more than once", e.CallID)
	}
	if e.ModelCallSequence > 0 {
		position := toolPosition{
			modelCallSequence: e.ModelCallSequence,
			toolCallIndex:     e.ToolCallIndex,
		}
		if existing, duplicate := r.toolPositions[position]; duplicate {
			return nil, fmt.Errorf(
				"tool call %q repeats model call %d ToolCall index %d already owned by %q",
				e.CallID,
				e.ModelCallSequence,
				e.ToolCallIndex,
				existing,
			)
		}
	}
	arguments, err := parseToolArguments(e.Arguments)
	if err != nil {
		return nil, fmt.Errorf("tool %q arguments: %w", e.ToolName, err)
	}
	if err := r.resume.rejectCommittedToolStart(e.CallID, e.ToolName, arguments); err != nil {
		return nil, err
	}
	out := r.closeStreaming()
	r.toolOrder++
	// The step number previews the Run's accounting, so it counts the same thing
	// the committed metrics do. Reporting the segment's own count would make a
	// resumed Run appear to start over at step 1.
	step := r.metrics().Steps
	out = append(out, SegmentProgressed{Progress: RunProgress{
		Step: &step, ToolName: e.ToolName, Activity: e.Activity,
	}})
	identity := r.reuseOrCreateToolItem(e.CallID, e.ToolName, arguments)
	ref := &openTool{
		callID: e.CallID, sourceCallID: e.SourceCallID, arrivalOrder: r.toolOrder,
		modelCallSequence: e.ModelCallSequence, toolCallIndex: e.ToolCallIndex,
		id: identity.id, occurredAt: identity.occurredAt, attemptStartedAt: r.now(),
		name: e.ToolName, arguments: arguments, safetyClass: e.SafetyClass,
	}
	r.toolCallIDs[e.CallID] = struct{}{}
	if e.ModelCallSequence > 0 {
		r.toolPositions[toolPosition{
			modelCallSequence: e.ModelCallSequence,
			toolCallIndex:     e.ToolCallIndex,
		}] = e.CallID
	}
	r.tools.add(ref)
	out = append(out, ItemStarted{Item: r.runningToolItem(ref)})
	if e.Arguments != "" {
		out = append(out, ItemChanged{
			ItemID: ref.id,
			Delta:  ItemDelta{Kind: ToolArgumentsDelta, ArgumentsTextDelta: e.Arguments},
		})
	}
	return out, nil
}

func (r *reducer) runningToolItem(ref *openTool) transcript.Item {
	return transcript.Item{
		ID: ref.id, RunID: r.cfg.RunID, Status: transcript.ItemRunning,
		Kind: transcript.ToolCall, OccurredAt: ref.occurredAt,
		Tool:        newToolInvocation(ref.name, ref.arguments, nil),
		SafetyClass: ref.safetyClass,
	}
}

func (r *reducer) openToolItemID(callID string) (string, bool) {
	ref, open := r.tools[callID]
	if !open || ref == nil {
		return "", false
	}
	return ref.id, true
}

// spawningItem resolves the executor's immutable parent-call identity to the
// canonical running Item that represents it. Only currently open calls are
// eligible: an AgentTool creates its child before that parent call can finish.
// It returns the complete canonical Item because child admission must persist
// that Item in the same transaction as the child's lineage edge. Ambiguity is
// rejected rather than resolved by ordering.
func (r *reducer) spawningItem(sourceCallID string) (transcript.Item, error) {
	if strings.TrimSpace(sourceCallID) == "" {
		return transcript.Item{}, errors.New("spawning source call id is required")
	}
	var match *openTool
	for _, candidate := range r.tools {
		if candidate.sourceCallID != sourceCallID {
			continue
		}
		if match != nil {
			return transcript.Item{}, fmt.Errorf("source call %q identifies multiple open tool items", sourceCallID)
		}
		match = candidate
	}
	if match == nil {
		return transcript.Item{}, fmt.Errorf("source call %q has no open tool item", sourceCallID)
	}
	return r.runningToolItem(match), nil
}

func (r *reducer) toolEnd(e ToolCallFinished) ([]RunEvent, []ToolInvocationCommit, error) {
	ref, ok := r.tools[e.CallID]
	if !ok {
		if consumed, err := r.resume.consumeCommittedTool(e); consumed {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("tool call %q ended without an open start", e.CallID)
	}
	if ref.end != nil {
		return nil, nil, fmt.Errorf("tool call %q ended more than once", e.CallID)
	}
	cloned := e
	if e.Offload != nil {
		ref := *e.Offload
		cloned.Offload = &ref
	}
	if e.Problem != nil {
		problem := *e.Problem
		cloned.Problem = &problem
	}
	cloned.MutatedPaths = slices.Clone(e.MutatedPaths)
	finishedAt := r.now()
	if finishedAt.Before(ref.attemptStartedAt) {
		return nil, nil, fmt.Errorf("tool call %q finish time precedes start time", e.CallID)
	}
	ref.finishedAt = finishedAt
	ref.end = &cloned
	return r.flushEndedTools()
}

// flushEndedTools commits only the longest completed prefix. Tools may finish
// concurrently in any order, but transcript identity, mutation nudges, and
// durable insertion order must follow the model's call order.
func (r *reducer) flushEndedTools() ([]RunEvent, []ToolInvocationCommit, error) {
	ordered := r.tools.ordered()
	var out []RunEvent
	var invocations []ToolInvocationCommit
	for _, ref := range ordered {
		if ref.end == nil {
			break
		}
		delete(r.tools, ref.callID)
		completed, err := r.completeTool(ref, *ref.end)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, completed...)
		if ref.modelCallSequence > 0 {
			invocations = append(invocations, ToolInvocationCommit{
				CallID: ref.callID, ItemID: ref.id, SegmentID: r.cfg.SegmentID,
				State: ToolInvocationCompleted, StartedAt: ref.attemptStartedAt, FinishedAt: ref.finishedAt,
			})
		}
	}
	return out, invocations, nil
}

// forgetToolEnds removes speculative external results whose canonical batch
// failed to commit. Their starts remain open so RunLost synthesis records
// incomplete calls rather than publishing results that persistence rejected.
func (r *reducer) forgetToolEnds(callIDs []string) {
	for _, callID := range callIDs {
		ref := r.tools[callID]
		if ref == nil {
			continue
		}
		ref.end = nil
		ref.finishedAt = time.Time{}
	}
}

func (r *reducer) completeTool(ref *openTool, e ToolCallFinished) ([]RunEvent, error) {
	var out []RunEvent
	if e.OutputText != "" {
		out = append(out, ItemChanged{
			ItemID: ref.id,
			Delta:  ItemDelta{Kind: ToolOutputDelta, Text: e.OutputText},
		})
	}
	arguments := ref.arguments
	if e.Arguments != "" {
		parsed, err := parseToolArguments(e.Arguments)
		if err != nil {
			return nil, fmt.Errorf("tool %q effective arguments: %w", ref.name, err)
		}
		arguments = parsed
	}
	invocation := newToolInvocation(ref.name, arguments, e.Result)
	invocation.Offload = e.Offload
	item := transcript.Item{
		ID: ref.id, RunID: r.cfg.RunID, Status: transcript.ItemCompleted,
		Kind: transcript.ToolCall, OccurredAt: ref.occurredAt, FinishedAt: ref.finishedAt,
		Tool:        invocation,
		SafetyClass: ref.safetyClass,
	}
	if e.Problem != nil {
		if err := e.Problem.ValidateFor(transcript.ToolProblem); err != nil {
			return nil, fmt.Errorf("tool %q problem: %w", ref.name, err)
		}
		item.Status = transcript.ItemIncomplete
		problem := *e.Problem
		item.Error = &problem
	}
	return append(out, ItemCompleted{Item: item, mutatedPaths: e.MutatedPaths}), nil
}

// usageProgress records the executor's latest accounting report and previews the
// Run's resulting total. The report is remembered rather than only forwarded:
// it is what the Run commits if the segment ends without a fresh one.
func (r *reducer) usageProgress(e UsageReported) ([]RunEvent, error) {
	if err := r.applyUsage(SegmentUsage{
		Tokens:  e.TokenUsage,
		ByModel: e.ByModel,
		CostUSD: e.CostUSD,
		Steps:   e.Steps,
	}); err != nil {
		return nil, err
	}
	progress := RunProgress{Usage: r.metrics().Usage}
	if e.ContextTokens > 0 {
		contextTokens := e.ContextTokens
		progress.ContextTokens = &contextTokens
	}
	step := r.step
	progress.Step = &step
	return []RunEvent{SegmentProgressed{Progress: progress}}, nil
}

func (r *reducer) compaction(e CompactionBoundary) []RunEvent {
	dropped := max(e.MessagesBefore-e.MessagesAfter, 0)
	id, now := r.nextItemID(), r.now()
	return itemPair(func(status transcript.ItemStatus) transcript.Item {
		return transcript.Item{
			ID: id, RunID: r.cfg.RunID, Status: status,
			Kind: transcript.Compaction, OccurredAt: now, DroppedMessages: dropped,
		}
	})
}

func (r *reducer) openUserMessage() []RunEvent {
	if len(r.userInput) == 0 {
		return nil
	}
	input := r.userInput
	r.userInput = nil
	id, now := userMessageItemID(r.cfg.SegmentID), r.now()
	return itemPair(func(status transcript.ItemStatus) transcript.Item {
		return transcript.Item{
			ID: id, RunID: r.cfg.RunID, Status: status,
			Kind: transcript.UserMessage, OccurredAt: now, Content: input,
		}
	})
}

func (r *reducer) steerMessagesApplied(e SteerMessagesApplied) ([]RunEvent, error) {
	if len(e.Messages) == 0 {
		return nil, errors.New("applied steer batch is empty")
	}
	out := r.closeStreaming()
	for messageIndex, message := range e.Messages {
		if len(message) == 0 {
			return nil, fmt.Errorf("applied steer message %d is empty", messageIndex)
		}
		for blockIndex, block := range message {
			if err := block.Validate(); err != nil {
				return nil, fmt.Errorf(
					"applied steer message %d content %d: %w",
					messageIndex, blockIndex, err,
				)
			}
		}
		id, now := r.nextItemID(), r.now()
		content := transcript.CloneContent(message)
		out = append(out, itemPair(func(status transcript.ItemStatus) transcript.Item {
			return transcript.Item{
				ID: id, RunID: r.cfg.RunID, Status: status,
				Kind: transcript.UserMessage, OccurredAt: now,
				Content: content,
			}
		})...)
	}
	return out, nil
}

func (r *reducer) planSnapshot(e PlanUpdated) []RunEvent {
	snapshot := r.planState(e.State)
	// Remembered so the segment can fence its final value: a client folding this
	// stream must reach segment.finished holding the state the segment ended with,
	// not the state as of whichever change happened to be published last.
	r.plan = &snapshot
	return []RunEvent{snapshot}
}

func (r *reducer) planState(state plan.State) StateSnapshot {
	current := make([]PlanSnapshot, len(state.Steps))
	for i, step := range state.Steps {
		current[i] = PlanSnapshot{
			ID: strconv.Itoa(i), Description: step.Description, Status: step.Status,
		}
	}
	return StateSnapshot{
		SessionID: r.cfg.SessionID, Plan: current,
		Revision: state.Revision, UpdatedAt: state.UpdatedAt,
	}
}
