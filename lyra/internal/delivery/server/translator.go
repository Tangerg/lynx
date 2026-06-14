package server

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/delivery/protocol"
	"github.com/Tangerg/lynx/lyra/internal/domain/interrupts"
	"github.com/Tangerg/lynx/lyra/internal/kernel/turn"
)

// resumeBindingFrom extracts the pending approval items' ids (keyed by tool
// name + arguments) from a parked run so the continuation translator can
// reuse them when the approved tools re-fire. Returns nil when there are no
// approval interrupts (e.g. a plan-review question, which resolves without a
// re-fired tool). originRunID is the interrupted run the items were created
// in — the continuation re-emits them under that run so the item's id + runId
// stay stable across the boundary.
func resumeBindingFrom(pending interrupts.Pending) *resumeBinding {
	var ints []protocol.Interrupt
	if err := json.Unmarshal(pending.Interrupts, &ints); err != nil || len(ints) == 0 {
		return nil
	}
	items := map[string]string{}
	byName := map[string]string{}
	// addItem indexes a proposal item both by (name, args) for an exact re-fire
	// match and by name alone for the edited-args fallback. A name shared by two
	// proposals is marked ambiguous ("") so the fallback won't guess.
	addItem := func(name, argsK, itemID string) {
		items[resumeKey(name, argsK)] = itemID
		if _, dup := byName[name]; dup {
			byName[name] = ""
		} else {
			byName[name] = itemID
		}
	}
	var questions []resumedQuestion
	for _, in := range ints {
		if in.ItemID == "" {
			continue
		}
		switch in.Type {
		case protocol.InterruptApproval:
			// Re-bind straight off payload.tool (API.md §4.8): the
			// domain-neutral ToolInvocation always carries name + arguments, so
			// the re-fired approved tool matches THIS proposal item by
			// (name, canonical arguments) — no backend-internal `_resume` tuple.
			tool, _ := in.Payload["tool"].(map[string]any)
			name, _ := tool["name"].(string)
			args, _ := tool["arguments"].(map[string]any)
			if name != "" {
				addItem(name, argsKey(args), in.ItemID)
			}
		case protocol.InterruptQuestion:
			// A plan-review question is resolved by the resume answer (no
			// re-fired event), so the continuation must complete its item.
			questions = append(questions, resumedQuestion{itemID: in.ItemID, question: questionFromPayload(in.Payload)})
		}
	}
	// Tools that were still open at park time (e.g. the ask_user call
	// that interrupted from inside its own execution) re-fire on resume
	// and must reuse their ORIGINAL item ids — typed bookkeeping on the
	// pending record, never part of the wire payload.
	for _, dt := range pending.DrainedTools {
		if dt.Name == "" || dt.ItemID == "" {
			continue
		}
		addItem(dt.Name, argsKey(protocol.ParseArgs(dt.Arguments)), dt.ItemID)
	}
	if len(items) == 0 && len(questions) == 0 {
		return nil
	}
	return &resumeBinding{originRunID: pending.ParentRunID, toolItems: items, byName: byName, questions: questions}
}

// questionFromPayload reconstructs the wire Question from an interrupt's
// payload map (round-tripped through JSON in the interrupt store) so the
// continuation's terminal item.completed carries the same content the
// proposal did. Returns nil when absent / malformed (the item still
// completes — just without re-stated content; the client already has it).
func questionFromPayload(payload map[string]any) *protocol.Question {
	raw, ok := payload["question"]
	if !ok {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var q protocol.Question
	if err := json.Unmarshal(b, &q); err != nil {
		return nil
	}
	return &q
}

// translator converts Lyra's internal [turn.Event] delta stream into
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
	byName      map[string]string // toolName → original item id; edited-args fallback ("" = ambiguous)
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
	parentRunID string           // non-empty for continuation runs (runs.resume)
	model       string           // run's model → RunRef.model on run.started
	mode        protocol.RunMode // run's mode → RunRef.mode on run.started
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

	// parkDrained is the snapshot of tool items that were still open
	// when the turn parked (set by [translator.interrupt]). The pump
	// records it on the pending interrupt as backend-private resume
	// bookkeeping — see [interrupts.Pending.DrainedTools].
	parkDrained []interrupts.DrainedTool

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

func newTranslator(sessionID, runID, parentRunID string, userInput []protocol.ContentBlock, resume *resumeBinding, model string, mode protocol.RunMode) *translator {
	return &translator{
		runID:       runID,
		sessionID:   sessionID,
		parentRunID: parentRunID,
		model:       model,
		mode:        mode,
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
// the domain-neutral envelope always carries name + arguments, so the
// resume binding reads them directly — keeping correlation simple.
// Null byte separator — tool names per the spec cannot contain it
// (lowercase alphanumerics + single hyphens only), so the join is
// unambiguous and collision-free.
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
// freshly minted id under the current run otherwise. Primary match is exact
// (name, arguments). When the user EDITED the args at approval the re-fire
// carries different args and misses that key, so it falls back to the unique
// proposal item for this tool name (the runtime parks one awaitable at a time)
// — otherwise the original proposal card would never get its terminal
// item.completed and would hang "in progress" forever.
func (t *translator) reuseOrNextItemID(toolName, argsJSON string) (id, runID string) {
	if t.resume != nil {
		key := resumeKey(toolName, argsKey(protocol.ParseArgs(argsJSON)))
		if orig, ok := t.resume.toolItems[key]; ok {
			delete(t.resume.toolItems, key)
			return orig, t.resume.originRunID
		}
		if orig, ok := t.resume.byName[toolName]; ok && orig != "" {
			delete(t.resume.byName, toolName)
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
			Model:       t.model,
			Mode:        t.mode,
			Status:      protocol.RunStatusRunning,
			CreatedAt:   time.Now().UTC(),
		},
	}}
	out = append(out, t.openUserMessage()...)
	return append(out, t.resumeQuestionCompletions()...)
}

// translate maps one Lyra chat event to zero or more StreamEvents.
func (t *translator) translate(ev turn.Event) []protocol.StreamEvent {
	switch e := ev.(type) {
	case turn.TurnStart:
		// run.started is emitted by open() at the start of every run segment
		// (so continuation runs get it too — they carry no chat.TurnStart),
		// not here. Nothing to do for the chat-level TurnStart.
		return nil
	case turn.MessageDelta:
		// Close any open reasoning before emitting text — reasoning and
		// text cannot be concurrently open (API.md §5: at most one
		// streaming item at a time).
		out := t.closeReasoning()
		return append(out, t.appendText(e.Text)...)
	case turn.ReasoningDelta:
		// Close any open text before emitting reasoning.
		out := t.closeText()
		return append(out, t.appendReasoning(e.Text)...)
	case turn.ToolCallStart:
		return t.toolStart(e)
	case turn.ToolCallEnd:
		return t.toolEnd(e)
	case turn.ErrorEvent:
		t.errMsg = e.Message
		return nil
	case turn.TurnInterrupted:
		return t.interrupt(e)
	case turn.TurnEnd:
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
