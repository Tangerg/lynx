package server

import (
	"encoding/json"
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
