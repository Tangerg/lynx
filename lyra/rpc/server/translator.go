package server

import (
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/service/chat"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// translator converts Lyra's internal [chat.Event] delta stream into
// the v2 [protocol.StreamEvent] / Item model (API.md §5). One
// translator per run — it carries the in-flight Item state (open
// agentMessage / reasoning / toolCall items) so the output is
// well-formed regardless of how the underlying deltas interleave.
//
// State machine:
//
//	chat.TurnStart      → run.started
//	chat.MessageDelta   → close reasoning + item.started(agentMessage,lazy) + item.delta(content)
//	chat.ReasoningDelta → close text + item.started(reasoning,lazy) + item.delta(reasoning)
//	chat.ToolCallStart  → close text+reasoning + item.started(toolCall) + item.delta(toolArguments)
//	chat.ToolCallEnd    → item.completed(toolCall)
//	chat.TurnEnd        → close open items + run.finished(outcome)
//	chat.TurnInterrupted → close open items + interrupt Item(s) + run.finished(outcome:interrupt)
//	chat.ErrorEvent     → captured, surfaced in run.finished(outcome:error)
//
// PlanGenerated / CompactBoundary / MemoryUpdated are not surfaced here:
// the plan rides the interrupt outcome (TurnInterrupted), and compaction
// / memory are internal housekeeping outside the durable Item history.
type translator struct {
	runID       string
	sessionID   string
	parentRunID string // non-empty for continuation runs (runs.resume)
	itemSeq     int

	// userInput is the run's opening user message, emitted as the first
	// Item (userMessage) right after run.started. Set only for root runs
	// (runs.start); empty for continuations (runs.resume carry no new
	// user turn).
	userInput []protocol.ContentBlock

	text      *openText
	reasoning *openText
	tools     map[string]*openTool // callID → in-flight toolCall item

	errMsg string
}

type openText struct {
	id        string
	createdAt time.Time
	buf       strings.Builder
}

type openTool struct {
	id        string
	createdAt time.Time
	name      string
	kind      protocol.ToolInvocationKind
}

func newTranslator(sessionID, runID, parentRunID string, userInput []protocol.ContentBlock) *translator {
	return &translator{
		runID:       runID,
		sessionID:   sessionID,
		parentRunID: parentRunID,
		userInput:   userInput,
		tools:       map[string]*openTool{},
	}
}

func (t *translator) nextItemID() string {
	t.itemSeq++
	return protocol.IDPrefixItem + t.runID + "_" + strconv.Itoa(t.itemSeq)
}

// userMessageItemID is the deterministic id of a run's opening userMessage
// Item — derived from the runId so [StartRun] can return it in the response
// (for optimistic-bubble reconciliation) and the translator stamps the same
// one onto the streamed + persisted Item. The "_u" suffix keeps it clear of
// the translator's numbered items (_1, _2, …).
func userMessageItemID(runID string) string {
	return protocol.IDPrefixItem + runID + "_u"
}

// translate maps one Lyra chat event to zero or more StreamEvents.
func (t *translator) translate(ev chat.Event) []protocol.StreamEvent {
	switch e := ev.(type) {
	case chat.TurnStart:
		out := []protocol.StreamEvent{{
			Type: protocol.StreamRunStarted,
			Run: &protocol.RunRef{
				ID:          t.runID,
				SessionID:   t.sessionID,
				ParentRunID: t.parentRunID,
				Status:      protocol.RunStatusRunning,
				CreatedAt:   time.Now().UTC(),
			},
		}}
		return append(out, t.openUserMessage()...)
	case chat.MessageDelta:
		out := t.closeReasoning()
		return append(out, t.appendText(e.Text)...)
	case chat.ReasoningDelta:
		out := t.closeText()
		return append(out, t.appendReasoning(e.Text)...)
	case chat.ToolCallStart:
		return t.toolStart(e)
	case chat.ToolCallEnd:
		return t.toolEnd(e)
	case chat.ErrorEvent:
		t.errMsg = e.Message
		return nil
	case chat.TurnInterrupted:
		return t.interrupt(e)
	case chat.TurnEnd:
		return t.turnEnd(e)
	}
	return nil
}

// openUserMessage emits the run's opening user turn as a userMessage Item
// (item.started + item.completed) so the live stream carries it — the
// client renders the user bubble straight from the event flow and learns
// its durable item id (matching items.list on reload). Emitted once: it
// consumes t.userInput. Empty for continuation runs.
func (t *translator) openUserMessage() []protocol.StreamEvent {
	if len(t.userInput) == 0 {
		return nil
	}
	input := t.userInput
	t.userInput = nil
	id := userMessageItemID(t.runID)
	now := time.Now().UTC()
	item := func(status protocol.ItemStatus) *protocol.Item {
		return &protocol.Item{
			ID:        id,
			RunID:     t.runID,
			Status:    status,
			Type:      protocol.ItemTypeUserMessage,
			CreatedAt: now,
			Content:   input,
		}
	}
	return []protocol.StreamEvent{
		{Type: protocol.StreamItemStarted, Item: item(protocol.ItemStatusInProgress)},
		{Type: protocol.StreamItemCompleted, Item: item(protocol.ItemStatusCompleted)},
	}
}

// interrupt maps a parked turn (HITL) onto its Item(s) + a terminal
// run.finished{outcome:interrupt}. Each pending awaitable becomes a
// durable Item the client renders plus a protocol.Interrupt keyed by
// that item's id:
//
//	approval → a toolCall Item (inProgress) for the gated call
//	question → a question Item (inProgress) for a plan awaiting review
//
// (The contract has no "plan" interrupt kind — plan-review rides the
// generic question mechanism; see questionInterrupt.)
func (t *translator) interrupt(e chat.TurnInterrupted) []protocol.StreamEvent {
	out := t.closeReasoning()
	out = append(out, t.closeText()...)

	wire := make([]protocol.Interrupt, 0, len(e.Interrupts))
	for _, in := range e.Interrupts {
		var ev protocol.StreamEvent
		var entry protocol.Interrupt
		switch in.Kind {
		case "approval":
			ev, entry = t.approvalInterrupt(in)
		default: // question (plan-review)
			ev, entry = t.questionInterrupt(in)
		}
		out = append(out, ev)
		wire = append(wire, entry)
	}

	return append(out, protocol.StreamEvent{
		Type:    protocol.StreamRunFinished,
		Outcome: &protocol.RunOutcome{Type: protocol.OutcomeInterrupt, Interrupts: wire},
	})
}

// approvalInterrupt renders a gated tool call awaiting approval as an
// inProgress toolCall Item plus the protocol.Interrupt keyed to it.
func (t *translator) approvalInterrupt(in chat.Interrupt) (protocol.StreamEvent, protocol.Interrupt) {
	p, _ := in.Payload.(chat.ApprovalPrompt)
	id := t.nextItemID()
	ev := protocol.StreamEvent{
		Type: protocol.StreamItemStarted,
		Item: &protocol.Item{
			ID:        id,
			RunID:     t.runID,
			Status:    protocol.ItemStatusInProgress,
			Type:      protocol.ItemTypeToolCall,
			CreatedAt: time.Now().UTC(),
			Tool:      &protocol.ToolInvocation{Kind: toolKind(p.ToolName), Name: p.ToolName},
		},
	}
	entry := protocol.Interrupt{
		ItemID:  id,
		Kind:    "approval",
		Payload: map[string]any{"tool": p.ToolName, "arguments": p.Arguments},
	}
	return ev, entry
}

// Plan-review interrupt shape. The contract has no "plan" interrupt kind
// (API.md §6: approval | question | toolResult), so a plan awaiting review
// surfaces through the generic question mechanism: an inProgress question
// Item whose prompt is the plan markdown and whose single choice field
// decides approve / reject. These constants are the single source the
// resume path (resolveDecision) reads the answer back against.
const (
	planDecisionField   = "decision"
	planDecisionApprove = "Approve"
	planDecisionReject  = "Reject"
)

// questionInterrupt renders a plan awaiting review as an inProgress
// question Item (the plan markdown as the prompt, an Approve/Reject choice)
// plus the protocol.Interrupt keyed to it. The client answers via
// runs.resume with an "answer" response carrying the chosen label.
func (t *translator) questionInterrupt(in chat.Interrupt) (protocol.StreamEvent, protocol.Interrupt) {
	plan, _ := in.Payload.(string)
	id := t.nextItemID()
	question := &protocol.Question{
		Prompt: plan,
		Fields: []protocol.QuestionField{{
			Name:     planDecisionField,
			Label:    "Proceed with this plan?",
			Header:   "Plan",
			Required: true,
			Type:     "choice",
			Options: []protocol.QuestionOption{
				{Label: planDecisionApprove},
				{Label: planDecisionReject},
			},
		}},
	}
	ev := protocol.StreamEvent{
		Type: protocol.StreamItemStarted,
		Item: &protocol.Item{
			ID:        id,
			RunID:     t.runID,
			Status:    protocol.ItemStatusInProgress,
			Type:      protocol.ItemTypeQuestion,
			CreatedAt: time.Now().UTC(),
			Question:  question,
		},
	}
	entry := protocol.Interrupt{
		ItemID:  id,
		Kind:    "question",
		Payload: map[string]any{"question": question},
	}
	return ev, entry
}

func (t *translator) appendText(text string) []protocol.StreamEvent {
	var out []protocol.StreamEvent
	if t.text == nil {
		t.text = &openText{id: t.nextItemID(), createdAt: time.Now().UTC()}
		out = append(out, protocol.StreamEvent{
			Type: protocol.StreamItemStarted,
			Item: &protocol.Item{
				ID:        t.text.id,
				RunID:     t.runID,
				Status:    protocol.ItemStatusInProgress,
				Type:      protocol.ItemTypeAgentMessage,
				CreatedAt: t.text.createdAt,
			},
		})
	}
	t.text.buf.WriteString(text)
	idx := 0
	return append(out, protocol.StreamEvent{
		Type:   protocol.StreamItemDelta,
		ItemID: t.text.id,
		Delta:  &protocol.ItemDelta{Type: protocol.DeltaContent, Index: &idx, Text: text},
	})
}

func (t *translator) appendReasoning(text string) []protocol.StreamEvent {
	var out []protocol.StreamEvent
	if t.reasoning == nil {
		t.reasoning = &openText{id: t.nextItemID(), createdAt: time.Now().UTC()}
		out = append(out, protocol.StreamEvent{
			Type: protocol.StreamItemStarted,
			Item: &protocol.Item{
				ID:        t.reasoning.id,
				RunID:     t.runID,
				Status:    protocol.ItemStatusInProgress,
				Type:      protocol.ItemTypeReasoning,
				CreatedAt: t.reasoning.createdAt,
			},
		})
	}
	t.reasoning.buf.WriteString(text)
	return append(out, protocol.StreamEvent{
		Type:   protocol.StreamItemDelta,
		ItemID: t.reasoning.id,
		Delta:  &protocol.ItemDelta{Type: protocol.DeltaReasoning, Text: text},
	})
}

func (t *translator) closeText() []protocol.StreamEvent {
	if t.text == nil {
		return nil
	}
	item := &protocol.Item{
		ID:        t.text.id,
		RunID:     t.runID,
		Status:    protocol.ItemStatusCompleted,
		Type:      protocol.ItemTypeAgentMessage,
		CreatedAt: t.text.createdAt,
		Content:   []protocol.ContentBlock{{Type: "text", Text: t.text.buf.String()}},
	}
	t.text = nil
	return []protocol.StreamEvent{{Type: protocol.StreamItemCompleted, Item: item}}
}

func (t *translator) closeReasoning() []protocol.StreamEvent {
	if t.reasoning == nil {
		return nil
	}
	item := &protocol.Item{
		ID:        t.reasoning.id,
		RunID:     t.runID,
		Status:    protocol.ItemStatusCompleted,
		Type:      protocol.ItemTypeReasoning,
		CreatedAt: t.reasoning.createdAt,
		Text:      t.reasoning.buf.String(),
	}
	t.reasoning = nil
	return []protocol.StreamEvent{{Type: protocol.StreamItemCompleted, Item: item}}
}

func (t *translator) toolStart(e chat.ToolCallStart) []protocol.StreamEvent {
	out := t.closeReasoning()
	out = append(out, t.closeText()...)

	ref := &openTool{id: t.nextItemID(), createdAt: time.Now().UTC(), name: e.ToolName, kind: toolKind(e.ToolName)}
	t.tools[e.CallID] = ref
	out = append(out, protocol.StreamEvent{
		Type: protocol.StreamItemStarted,
		Item: &protocol.Item{
			ID:        ref.id,
			RunID:     t.runID,
			Status:    protocol.ItemStatusInProgress,
			Type:      protocol.ItemTypeToolCall,
			CreatedAt: ref.createdAt,
			Tool:      &protocol.ToolInvocation{Kind: ref.kind, Name: ref.name},
		},
	})
	if e.Arguments != "" {
		out = append(out, protocol.StreamEvent{
			Type:   protocol.StreamItemDelta,
			ItemID: ref.id,
			Delta:  &protocol.ItemDelta{Type: protocol.DeltaToolArguments, ArgumentsTextDelta: e.Arguments},
		})
	}
	return out
}

func (t *translator) toolEnd(e chat.ToolCallEnd) []protocol.StreamEvent {
	ref, ok := t.tools[e.CallID]
	if !ok {
		return nil
	}
	delete(t.tools, e.CallID)
	item := &protocol.Item{
		ID:        ref.id,
		RunID:     t.runID,
		Status:    protocol.ItemStatusCompleted,
		Type:      protocol.ItemTypeToolCall,
		CreatedAt: ref.createdAt,
		Tool:      &protocol.ToolInvocation{Kind: ref.kind, Name: ref.name, Output: e.Output},
	}
	if e.Err != "" {
		item.Status = protocol.ItemStatusIncomplete
		item.Error = &protocol.ProblemData{Type: "tool_failed", Detail: e.Err}
	}
	return []protocol.StreamEvent{{Type: protocol.StreamItemCompleted, Item: item}}
}

// turnEnd closes any open items (so the wire ends balanced) then emits
// the terminal run.finished with its discriminated outcome.
func (t *translator) turnEnd(e chat.TurnEnd) []protocol.StreamEvent {
	out := t.closeReasoning()
	out = append(out, t.closeText()...)
	out = append(out, t.drainTools()...)
	return append(out, protocol.StreamEvent{
		Type:    protocol.StreamRunFinished,
		Outcome: t.outcome(e),
	})
}

// finish builds a terminal run.finished for paths that never observe a
// chat.TurnEnd (e.g. run cancellation drained the iterator). Closes
// open items, then emits run.finished with the given outcome type.
func (t *translator) finish(outcomeType protocol.RunOutcomeType) []protocol.StreamEvent {
	out := t.closeReasoning()
	out = append(out, t.closeText()...)
	out = append(out, t.drainTools()...)
	res := &protocol.RunResult{}
	if outcomeType == protocol.OutcomeError && t.errMsg != "" {
		res.Error = &protocol.ProblemData{Type: "internal_error", Detail: t.errMsg}
	}
	return append(out, protocol.StreamEvent{
		Type:    protocol.StreamRunFinished,
		Outcome: &protocol.RunOutcome{Type: outcomeType, Result: res},
	})
}

func (t *translator) drainTools() []protocol.StreamEvent {
	if len(t.tools) == 0 {
		return nil
	}
	out := make([]protocol.StreamEvent, 0, len(t.tools))
	for callID, ref := range t.tools {
		out = append(out, protocol.StreamEvent{
			Type: protocol.StreamItemCompleted,
			Item: &protocol.Item{
				ID:        ref.id,
				RunID:     t.runID,
				Status:    protocol.ItemStatusIncomplete,
				Type:      protocol.ItemTypeToolCall,
				CreatedAt: ref.createdAt,
				Tool:      &protocol.ToolInvocation{Kind: ref.kind, Name: ref.name},
			},
		})
		delete(t.tools, callID)
	}
	return out
}

func (t *translator) outcome(e chat.TurnEnd) *protocol.RunOutcome {
	res := &protocol.RunResult{
		Usage:   turnUsage(e),
		CostUSD: optCostUSD(e.CostUSD),
	}
	switch e.Reason {
	case chat.TurnEndCancelled:
		return &protocol.RunOutcome{Type: protocol.OutcomeCanceled, Result: res}
	case chat.TurnEndBudgetExceeded:
		return &protocol.RunOutcome{Type: protocol.OutcomeMaxBudget, Result: res}
	case chat.TurnEndErrored:
		detail := t.errMsg
		if detail == "" {
			detail = "turn errored"
		}
		res.Error = &protocol.ProblemData{Type: "internal_error", Detail: detail}
		return &protocol.RunOutcome{Type: protocol.OutcomeError, Result: res}
	default:
		return &protocol.RunOutcome{Type: protocol.OutcomeCompleted, Result: res}
	}
}

// turnUsage maps the engine's per-turn token roll-up onto wire Usage.
func turnUsage(e chat.TurnEnd) *protocol.Usage {
	u := &protocol.Usage{
		InputTokens:     e.TokenUsage.PromptTokens,
		OutputTokens:    e.TokenUsage.CompletionTokens,
		ReasoningTokens: e.TokenUsage.ReasoningTokens,
	}
	if len(e.UsageByModel) > 0 {
		u.ByModel = make(map[string]protocol.ModelUsage, len(e.UsageByModel))
		for _, m := range e.UsageByModel {
			u.ByModel[m.Model] = protocol.ModelUsage{
				InputTokens:  m.PromptTokens,
				OutputTokens: m.CompletionTokens,
				CostUSD:      optCostUSD(m.CostUSD),
			}
		}
	}
	return u
}

// optCostUSD returns &c only when a pricing hook produced a real figure
// (c > 0), else nil — API.md §4.2 omits cost rather than faking 0.
func optCostUSD(c float64) *float64 {
	if c > 0 {
		return &c
	}
	return nil
}

// toolKind maps a tool name onto the wire ToolInvocation kind for
// rendering (API.md §4.4). Best-effort by name; unknown tools render as
// command.
func toolKind(name string) protocol.ToolInvocationKind {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "mcp"):
		return protocol.ToolKindMCP
	case strings.Contains(n, "edit"), strings.Contains(n, "write"), strings.Contains(n, "patch"):
		return protocol.ToolKindFileEdit
	case strings.Contains(n, "search"), strings.Contains(n, "grep"), strings.Contains(n, "glob"),
		strings.Contains(n, "find"), strings.Contains(n, "web"):
		return protocol.ToolKindSearch
	default:
		return protocol.ToolKindCommand
	}
}
