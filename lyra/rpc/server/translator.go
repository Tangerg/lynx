package server

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/engine"
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
// resumeBinding carries a parked run's pending toolCall item ids into its
// continuation translator. When a continuation resumes an approved tool, the
// tool re-fires and the translator reuses its ORIGINAL proposal item id (and
// the run it was created in) instead of minting a fresh one — so one item
// flows proposal → approval → execution → completion, and the proposal item
// gets its mandatory terminal item.completed (API.md §5.2 / §6: the itemId is
// the correlation key across the interrupt boundary). nil for root runs.
type resumeBinding struct {
	originRunID string            // the interrupted run the items were created in
	toolItems   map[string]string // resumeKey(toolName, arguments) → original item id
	questions   []resumedQuestion // plan-review question items awaiting their terminal
}

// resumedQuestion is a plan-review question item from the interrupted run.
// Unlike a toolCall (which re-fires and completes on execution), a question
// is resolved by the resume answer itself — no event re-fires — so the
// continuation must emit its terminal item.completed explicitly (API.md §5.2)
// to close the proposal card. The Question payload is carried so the
// persisted completed item (items.list) keeps its content.
type resumedQuestion struct {
	itemID   string
	question *protocol.Question
}

type translator struct {
	runID       string
	sessionID   string
	parentRunID string // non-empty for continuation runs (runs.resume)
	resume      *resumeBinding
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
	runID     string // the run the item belongs to (origin run for a resumed tool)
	createdAt time.Time
	name      string
	args      string // raw JSON arguments, replayed to rebuild the invocation at completion
}

func newTranslator(sessionID, runID, parentRunID string, userInput []protocol.ContentBlock, resume *resumeBinding) *translator {
	return &translator{
		runID:       runID,
		sessionID:   sessionID,
		parentRunID: parentRunID,
		resume:      resume,
		userInput:   userInput,
		tools:       map[string]*openTool{},
	}
}

// resumeKey identifies a gated tool call by (name, canonical-arguments) — the
// same pair the approval gate keys its verdict on, so a re-fired approved call
// matches the pending item recorded at interrupt time. argsKey is the
// CANONICAL form of the arguments object: both the re-fire side (raw JSON
// string → parse → marshal) and the resume side (the round-tripped
// payload.tool.arguments map → marshal) produce the same string, since
// encoding/json sorts map keys deterministically. This is what lets the
// resume binding read (name, arguments) straight off payload.tool (§4.4) —
// the domain-neutral envelope always carries them — instead of the old
// backend-internal `_resume` tuple the strongly-typed variants forced.
func resumeKey(toolName, argsKey string) string {
	return toolName + "\x00" + argsKey
}

// argsKey is the canonical (key-sorted) JSON of a parsed arguments object,
// used as the stable half of resumeKey. nil args canonicalize to "null", an
// empty object to "{}" — consistently on both sides of the resume boundary.
func argsKey(args map[string]any) string {
	b, _ := json.Marshal(args)
	return string(b)
}

// reuseOrNextItemID returns the original proposal item id + its origin run for
// a re-fired approved tool (so the continuation completes the SAME item), or a
// freshly minted id under the current run otherwise. Matching is by
// (name, arguments); an edited-args approval won't match and cleanly falls
// back to a new item.
func (t *translator) reuseOrNextItemID(toolName, argsJSON string) (id, runID string) {
	if t.resume != nil {
		key := resumeKey(toolName, argsKey(parseArgs(argsJSON)))
		if orig, ok := t.resume.toolItems[key]; ok {
			delete(t.resume.toolItems, key)
			return orig, t.resume.originRunID
		}
	}
	return t.nextItemID(), t.runID
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

// open is the first thing emitted on EVERY run segment — root and
// continuation alike. It guarantees run.started leads the stream (the
// client's run boundary; continuation runs carry parentRunId), then the
// root's opening userMessage Item and, for a resumed run, the terminal
// item.completed for any plan-review question the parked run left open.
// Driven by pumpRun before any chat event, so it never depends on a
// chat-level TurnStart (which continuations don't emit).
func (t *translator) open() []protocol.StreamEvent {
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
	out = append(out, t.openUserMessage()...)
	return append(out, t.resumeQuestionCompletions()...)
}

// translate maps one Lyra chat event to zero or more StreamEvents.
func (t *translator) translate(ev chat.Event) []protocol.StreamEvent {
	switch e := ev.(type) {
	case chat.TurnStart:
		// run.started is emitted by open() at the start of every run segment
		// (so continuation runs get it too — they carry no chat.TurnStart),
		// not here. Nothing to do for the chat-level TurnStart.
		return nil
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
		{Type: protocol.StreamItemStarted, Item: item(protocol.ItemStatusRunning)},
		{Type: protocol.StreamItemCompleted, Item: item(protocol.ItemStatusCompleted)},
	}
}

// resumeQuestionCompletions terminalizes the plan-review question items the
// interrupted run left inProgress. A question is resolved by the resume
// answer (no event re-fires), so the continuation must emit its
// item.completed itself — otherwise the proposal card stays "LIVE" forever
// (API.md §5.2). Emitted once, right after run.started; the completed item
// keeps the original id + origin runId and carries the Question payload so
// items.list stays well-formed. No-op for root runs / tool-only resumes.
func (t *translator) resumeQuestionCompletions() []protocol.StreamEvent {
	if t.resume == nil || len(t.resume.questions) == 0 {
		return nil
	}
	out := make([]protocol.StreamEvent, 0, len(t.resume.questions))
	for _, q := range t.resume.questions {
		out = append(out, protocol.StreamEvent{
			Type: protocol.StreamItemCompleted,
			Item: &protocol.Item{
				ID:        q.itemID,
				RunID:     t.resume.originRunID,
				Status:    protocol.ItemStatusCompleted,
				Type:      protocol.ItemTypeQuestion,
				CreatedAt: time.Now().UTC(),
				Question:  q.question,
			},
		})
	}
	t.resume.questions = nil // emit once
	return out
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
	// Close any tool item still open when the turn parks (defensive: the
	// gated call itself paused before item.started, but a sibling tool could
	// be mid-flight) so no started item is left without a terminal (§5.2).
	out = append(out, t.drainTools()...)

	wire := make([]protocol.Interrupt, 0, len(e.Interrupts))
	for _, in := range e.Interrupts {
		var ev protocol.StreamEvent
		var entry protocol.Interrupt
		switch in.Kind {
		case "approval":
			ev, entry = t.approvalInterrupt(in)
		default: // question — ask_user (free text) or plan review (choice)
			if _, ok := in.Payload.(engine.QuestionPrompt); ok {
				ev, entry = t.askUserInterrupt(in)
			} else {
				ev, entry = t.questionInterrupt(in)
			}
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
	// The gated tool as a full ToolInvocation (arguments parsed, no result
	// yet). The approval Interrupt's payload reuses it (API.md §4.8:
	// ApprovalPayload.tool), so the client reads payload.tool directly
	// instead of guessing where the command / args live.
	inv := toolInvocation(p.ToolName, p.Arguments, "")
	ev := protocol.StreamEvent{
		Type: protocol.StreamItemStarted,
		Item: &protocol.Item{
			ID:        id,
			RunID:     t.runID,
			Status:    protocol.ItemStatusRunning,
			Type:      protocol.ItemTypeToolCall,
			CreatedAt: time.Now().UTC(),
			Tool:      inv,
		},
	}
	entry := protocol.Interrupt{
		ItemID: id,
		Type:   "approval",
		// payload.tool is the self-contained ApprovalPayload (API.md §4.8): the
		// domain-neutral ToolInvocation always carries name + arguments, so the
		// server re-binds the re-fired approved tool to THIS proposal item
		// across the resume boundary straight off payload.tool (resumeKey on
		// name + canonical arguments) — no backend-internal `_resume` tuple.
		Payload: map[string]any{"tool": inv},
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
			Status:    protocol.ItemStatusRunning,
			Type:      protocol.ItemTypeQuestion,
			CreatedAt: time.Now().UTC(),
			Question:  question,
		},
	}
	entry := protocol.Interrupt{
		ItemID:  id,
		Type:    "question",
		Payload: map[string]any{"question": question},
	}
	return ev, entry
}

// askUserQuestionField is the free-text answer field name the ask_user
// tool reads back (engine.answerText keys on "text"); the resume "answer"
// response carries answers["text"].
const askUserQuestionField = "text"

// askUserInterrupt renders the model's ask_user call as an inProgress
// question Item carrying the actual question + a single free-text answer
// field (vs. questionInterrupt's plan Approve/Reject choice). The client
// answers via runs.resume with an "answer" response carrying the text.
func (t *translator) askUserInterrupt(in chat.Interrupt) (protocol.StreamEvent, protocol.Interrupt) {
	q, _ := in.Payload.(engine.QuestionPrompt)
	id := t.nextItemID()
	question := &protocol.Question{
		Prompt: q.Question,
		Fields: []protocol.QuestionField{{
			Name:     askUserQuestionField,
			Label:    q.Question,
			Required: true,
			Type:     "text",
		}},
	}
	ev := protocol.StreamEvent{
		Type: protocol.StreamItemStarted,
		Item: &protocol.Item{
			ID:        id,
			RunID:     t.runID,
			Status:    protocol.ItemStatusRunning,
			Type:      protocol.ItemTypeQuestion,
			CreatedAt: time.Now().UTC(),
			Question:  question,
		},
	}
	entry := protocol.Interrupt{
		ItemID:  id,
		Type:    "question",
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
				Status:    protocol.ItemStatusRunning,
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
				Status:    protocol.ItemStatusRunning,
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

	id, runID := t.reuseOrNextItemID(e.ToolName, e.Arguments)
	ref := &openTool{id: id, runID: runID, createdAt: time.Now().UTC(), name: e.ToolName, args: e.Arguments}
	t.tools[e.CallID] = ref
	out = append(out, protocol.StreamEvent{
		Type: protocol.StreamItemStarted,
		Item: &protocol.Item{
			ID:        ref.id,
			RunID:     ref.runID,
			Status:    protocol.ItemStatusRunning,
			Type:      protocol.ItemTypeToolCall,
			CreatedAt: ref.createdAt,
			Tool:      toolInvocation(e.ToolName, e.Arguments, ""),
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

	var out []protocol.StreamEvent
	// The authoritative command output lands on the completed item's
	// tool.result.output (durable, below); this toolOutput delta is only its
	// streaming preview (API.md §4.4 / §5.2). Same merged stdout+stderr text
	// so preview and terminal agree.
	if isCommandTool(ref.name) && e.Output != "" {
		if merged := commandOutput(e.Output); merged != "" {
			out = append(out, protocol.StreamEvent{
				Type:   protocol.StreamItemDelta,
				ItemID: ref.id,
				Delta:  &protocol.ItemDelta{Type: protocol.DeltaToolOutput, Text: merged},
			})
		}
	}

	item := &protocol.Item{
		ID:        ref.id,
		RunID:     ref.runID,
		Status:    protocol.ItemStatusCompleted,
		Type:      protocol.ItemTypeToolCall,
		CreatedAt: ref.createdAt,
		Tool:      toolInvocation(ref.name, ref.args, e.Output),
	}
	switch {
	case e.Denied:
		// Denied by the approval verdict — a distinct terminal from a green
		// success or a generic failure, so the UI can render "denied".
		item.Status = protocol.ItemStatusIncomplete
		item.Error = &protocol.ProblemData{Type: "denied_by_user", Detail: "tool call denied by user"}
	case e.Err != "":
		item.Status = protocol.ItemStatusIncomplete
		item.Error = &protocol.ProblemData{Type: "tool_failed", Detail: e.Err}
	}
	return append(out, protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: item})
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
		res.Error = classifyRunError(t.errMsg)
	}
	return append(out, protocol.StreamEvent{
		Type:    protocol.StreamRunFinished,
		Outcome: &protocol.RunOutcome{Type: outcomeType, Result: res},
	})
}

// internalErrorProblem builds the wire ProblemData for a run that failed
// with an internal error. The detail is a clean, generic message — the full
// error (with any wrapped Go context) rides the server-side turn span, never
// the wire (API.md §8.2: detail is a user/agent-readable note, not an
// implementation call path). After tool failures stopped escalating to run
// errors (FeedbackOnToolError), this path is genuine engine/infra failure.
func internalErrorProblem() *protocol.ProblemData {
	return &protocol.ProblemData{Type: "internal_error", Detail: "the run failed due to an internal error"}
}

// classifyRunError maps a failed run's (server-side, full) error message
// onto a wire ProblemData. Errors that originate at the model provider —
// rate limits, provider 5xx, auth/bad-request, timeouts/network — surface
// as a distinct provider_error (API.md §8.2, code -32001) with an
// actionable but NON-leaking detail (a fixed per-class string, never the
// raw message / URL / Go call path), so the client can react (back off on a
// rate limit, prompt for a key on auth) instead of treating every transient
// blip as an opaque internal_error and hammer-retrying. Anything
// unrecognized stays internal_error. The raw message still rides only the
// server-side span.
func classifyRunError(msg string) *protocol.ProblemData {
	m := strings.ToLower(msg)
	contains := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(m, s) {
				return true
			}
		}
		return false
	}
	provider := func(detail string) *protocol.ProblemData {
		return &protocol.ProblemData{Type: "provider_error", Detail: detail}
	}
	switch {
	case contains("429", "too many requests", "rate limit", "overloaded", "quota"):
		return provider("the model provider rate-limited the request; retry shortly")
	case contains(" 401", " 403", "unauthorized", "forbidden", "invalid_api_key", "api key"):
		return provider("the model provider rejected the credentials; check the provider API key")
	case contains(" 500", " 502", " 503", " 504", "bad gateway", "service unavailable", "internal server error"):
		return provider("the model provider is temporarily unavailable; retry shortly")
	case contains("deadline exceeded", "timeout", "timed out", "client.timeout", "connection refused", "no such host", "i/o timeout", "eof", "connection reset"):
		return provider("the model provider request timed out or the connection failed; retry shortly")
	case contains(" 400", "invalid_request_error", "bad request"):
		return provider("the model provider rejected the request as invalid")
	default:
		return internalErrorProblem()
	}
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
				RunID:     ref.runID,
				Status:    protocol.ItemStatusIncomplete,
				Type:      protocol.ItemTypeToolCall,
				CreatedAt: ref.createdAt,
				Tool:      toolInvocation(ref.name, ref.args, ""),
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
		res.Error = classifyRunError(t.errMsg)
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

// isCommandTool reports whether a tool name is a shell/command tool, whose
// stdout streams as a toolOutput preview and whose result is the command
// shape {exitCode, output, outputTruncated?} (§4.4.2). The name is the sole
// identity now (no kind on the wire, §4.4) — the display convention keys off
// it. Everything else falls through to a generic best-effort JSON result.
func isCommandTool(name string) bool {
	switch strings.ToLower(name) {
	case "bash", "shell":
		return true
	default:
		return false
	}
}

// toolInvocation builds the domain-neutral wire ToolInvocation for a tool
// call (API.md §4.4): Name (identity) + Arguments (parsed object, always
// present) + a best-effort JSON Result. Result is shaped per the §4.4.2
// display convention keyed by Name (bash→{exitCode,output,…}, grep/glob→
// {hits}, webSearch→{results}, edit/write→{changes}); any other tool gets a
// generic best-effort JSON result. argsJSON is the model's raw JSON
// arguments; outputJSON is the tool's JSON result ("" before completion, so
// the started shell carries no result). Tool failure / the streaming output
// preview are handled by the caller (toolEnd error mapping, toolOutput delta).
func toolInvocation(name, argsJSON, outputJSON string) *protocol.ToolInvocation {
	args := parseArgs(argsJSON)
	if args == nil {
		args = map[string]any{} // arguments is ALWAYS an object on the wire (§4.4.1)
	}
	inv := &protocol.ToolInvocation{Name: name, Arguments: args}
	if outputJSON != "" {
		inv.Result = toolResult(name, args, outputJSON)
	}
	return inv
}

// toolResult shapes a completed tool's best-effort JSON result per the
// §4.4.2 display convention keyed by tool name. The convention is
// non-normative (the client's display registry reads it) — an unknown tool
// falls back to a generic JSON value, which the client renders as a JSON
// tree. Failure never lands here (it rides toolCall.error, §4.3).
func toolResult(name string, args map[string]any, outputJSON string) any {
	switch strings.ToLower(name) {
	case "bash", "shell":
		return commandResultFrom(outputJSON)
	case "grep", "glob":
		return searchResult{Hits: parseLocalSearchHits(outputJSON)}
	case "websearch":
		return webSearchResultSet{Results: parseWebSearchHits(outputJSON)}
	case "write", "edit":
		if path := argString(args, "path"); path != "" {
			return fileChangeResult{Changes: []protocol.FileEdit{{Path: path, Status: "modified"}}}
		}
		return bestEffortJSON(outputJSON)
	default:
		return bestEffortJSON(outputJSON)
	}
}

// Result envelopes for the §4.4.2 display convention. Typed (not ad-hoc
// maps) so the shape is compile-time exact and marshals to the documented
// keys. They are server-side projection helpers, not protocol types — the
// reusable members (SearchHit / WebSearchResult / FileEdit) are.
type (
	// commandResult is the bash/shell convention: merged stdout+stderr as the
	// AUTHORITATIVE `output` (§5.2 — the toolOutput delta is only its preview)
	// + exitCode; outputTruncated flags a size-capped output. output is always
	// present (even ""), so history replay / reconnect (no delta) still renders
	// the terminal output rather than "(no output)".
	commandResult struct {
		ExitCode        *int   `json:"exitCode,omitempty"`
		Output          string `json:"output"`
		OutputTruncated bool   `json:"outputTruncated,omitempty"`
	}
	searchResult struct {
		Hits []protocol.SearchHit `json:"hits"`
	}
	webSearchResultSet struct {
		Results []protocol.WebSearchResult `json:"results"`
	}
	fileChangeResult struct {
		Changes []protocol.FileEdit `json:"changes"`
	}
)

// argString reads a string field from the parsed arguments, "" when absent.
func argString(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return s
}

// bestEffortJSON decodes raw as JSON (object / array / scalar) for a generic
// tool's result; when raw isn't valid JSON it's surfaced verbatim as a string
// (API.md §4.4: result is best-effort JSON, never double-encoded).
func bestEffortJSON(raw string) any {
	if raw == "" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw
	}
	return v
}

// commandResultFrom builds the command result from a bash tool's JSON output
// ({stdout, stderr, exit_code}): the AUTHORITATIVE merged `output` + exitCode
// (API.md §4.4.2 / §5.2). Output is always set — even "" — so a no-output
// command renders an empty terminal, not a stale preview.
func commandResultFrom(outputJSON string) commandResult {
	r := commandResult{Output: commandOutput(outputJSON)}
	var out struct {
		ExitCode int `json:"exit_code"`
	}
	if json.Unmarshal([]byte(outputJSON), &out) == nil {
		ec := out.ExitCode
		r.ExitCode = &ec
	}
	return r
}

// commandOutput merges a bash tool's stdout+stderr into the single full-text
// `output` value (API.md §4.4.2). The wire field is one stream (terminals
// interleave the two); lacking true interleave order we append stderr after
// stdout, separated by a newline when both are present.
func commandOutput(outputJSON string) string {
	var out struct {
		Stdout string `json:"stdout"`
		Stderr string `json:"stderr"`
	}
	_ = json.Unmarshal([]byte(outputJSON), &out)
	switch {
	case out.Stderr == "":
		return out.Stdout
	case out.Stdout == "":
		return out.Stderr
	default:
		return out.Stdout + "\n" + out.Stderr
	}
}

// parseLocalSearchHits maps grep / glob JSON output onto SearchHit values
// (a search tool's result {hits}, §4.4.2). grep "content" mode →
// {matches:[{path,line,text}]}; grep "files_with_matches" / glob →
// {files|paths:[…]}; counts mode → {counts}.
func parseLocalSearchHits(outputJSON string) []protocol.SearchHit {
	var out struct {
		Matches []struct {
			Path string `json:"path"`
			Line int    `json:"line"`
			Text string `json:"text"`
		} `json:"matches"`
		Files  []string `json:"files"`
		Paths  []string `json:"paths"`
		Counts []struct {
			Path  string `json:"path"`
			Count int    `json:"count"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(outputJSON), &out); err != nil {
		return nil
	}
	var hits []protocol.SearchHit
	for _, m := range out.Matches {
		hits = append(hits, protocol.SearchHit{Path: m.Path, LineNumber: m.Line, Snippet: m.Text})
	}
	for _, p := range append(out.Files, out.Paths...) {
		hits = append(hits, protocol.SearchHit{Path: p})
	}
	for _, c := range out.Counts {
		hits = append(hits, protocol.SearchHit{Path: c.Path, Snippet: strconv.Itoa(c.Count) + " matches"})
	}
	return hits
}

// parseWebSearchHits maps websearch JSON output ({results:[{title,url,
// snippet,favicon_url}]}) onto WebSearchResult values (a webSearch tool's
// result {results}, §4.4.2).
func parseWebSearchHits(outputJSON string) []protocol.WebSearchResult {
	var out struct {
		Results []struct {
			Title      string `json:"title"`
			URL        string `json:"url"`
			Snippet    string `json:"snippet"`
			FaviconURL string `json:"favicon_url"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(outputJSON), &out); err != nil {
		return nil
	}
	hits := make([]protocol.WebSearchResult, 0, len(out.Results))
	for _, r := range out.Results {
		hits = append(hits, protocol.WebSearchResult{Title: r.Title, URL: r.URL, Snippet: r.Snippet, FaviconURL: r.FaviconURL})
	}
	return hits
}
