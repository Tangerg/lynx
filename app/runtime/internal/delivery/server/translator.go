package server

import (
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
)

// translator converts Lyra's internal [turn.Event] delta stream into
// the v2 [protocol.StreamEvent] / Item model (API.md §5). One
// translator per run — it carries the in-flight Item state (open
// agentMessage / reasoning / toolCall items) so the output is
// well-formed regardless of how the underlying deltas interleave.
//
// State machine:
//
//	turn.TurnStart       → run.started
//	turn.MessageDelta    → close reasoning + item.started(agentMessage,lazy) + item.delta(content)
//	turn.ReasoningDelta  → close text + item.started(reasoning,lazy) + item.delta(reasoning)
//	turn.ToolCallStart   → close text+reasoning + item.started(toolCall) + item.delta(toolArguments)
//	turn.ToolCallEnd     → item.completed(toolCall)
//	turn.TurnEnd         → close open items + run.finished(outcome)
//	turn.TurnInterrupted → close open items + interrupt Item(s) + run.finished(outcome:interrupt)
//	turn.ErrorEvent      → captured, surfaced in run.finished(outcome:error)
//	turn.CompactBoundary → compaction Item (item.started + item.completed)
//
// MemoryUpdated is not surfaced here: extracted long-term memory is internal
// housekeeping with no client-facing surface (nothing folds a memory event).
type translator struct {
	runID       string
	sessionID   string
	parentRunID string // non-empty for continuation runs (runs.resume)
	provider    string // run's provider → RunRef.provider on run.started
	model       string // run's model → RunRef.model on run.started
	resume      *resumeBinding
	itemSeq     int
	step        int // tool-call ordinal, surfaced as run.progress.step (API.md §5)

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

	errMsg  string
	errCode string // turn-layer error code (AGENT_STUCK / ENGINE_ERROR / …) — classifies the run error
}

type openText struct {
	id        string
	createdAt time.Time
	buf       strings.Builder
}

type openTool struct {
	id          string
	runID       string // the run the item belongs to (origin run for a resumed tool)
	createdAt   time.Time
	name        string
	args        string // raw JSON arguments, replayed to rebuild the invocation at completion
	safetyClass string // wire SafetyClass, carried so item.completed matches item.started
}

func newTranslator(sessionID, runID, parentRunID string, userInput []protocol.ContentBlock, resume *resumeBinding, provider, model string) *translator {
	return &translator{
		runID:       runID,
		sessionID:   sessionID,
		parentRunID: parentRunID,
		provider:    provider,
		model:       model,
		resume:      resume,
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

// open is the first thing emitted on EVERY run segment — root and
// continuation alike. It guarantees run.started leads the stream (the
// client's run boundary; continuation runs carry parentRunId), then the
// root's opening userMessage Item and, for a resumed run, the terminal
// item.completed for any question the parked run left open. Driven by
// pumpRun before any turn event, so it never depends on a turn-level
// TurnStart (which continuations don't emit).
func (t *translator) open() []protocol.StreamEvent {
	out := []protocol.StreamEvent{{
		Type: protocol.StreamRunStarted,
		Run: &protocol.RunRef{
			ID:          t.runID,
			SessionID:   t.sessionID,
			ParentRunID: t.parentRunID,
			Provider:    t.provider,
			Model:       t.model,
			Status:      protocol.RunStatusRunning,
			CreatedAt:   time.Now().UTC(),
		},
	}}
	out = append(out, t.openUserMessage()...)
	return append(out, t.resumeQuestionCompletions()...)
}

// translate maps one Lyra turn event to zero or more StreamEvents.
func (t *translator) translate(ev turn.Event) []protocol.StreamEvent {
	switch e := ev.(type) {
	case turn.TurnStart:
		// run.started is emitted by open() at the start of every run segment
		// (so continuation runs get it too — they carry no turn.TurnStart),
		// not here. Nothing to do for the turn-level TurnStart.
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
	case turn.UsageReported:
		return t.usageProgress(e)
	case turn.SteerMessage:
		return t.steerMessage(e)
	case turn.TodosUpdated:
		return t.todosSnapshot(e)
	case turn.ErrorEvent:
		t.errMsg = e.Message
		t.errCode = e.Code
		return nil
	case turn.CompactBoundary:
		return t.compaction(e)
	case turn.TurnInterrupted:
		return t.interrupt(e)
	case turn.TurnEnd:
		return t.turnEnd(e)
	}
	return nil
}
