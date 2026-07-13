package server

import (
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// translator converts the application's normalized [runs.EngineEvent] stream into
// the v2 [protocol.StreamEvent] / Item model (API.md §5). One
// translator per SEGMENT — it carries the in-flight Item state (open
// agentMessage / reasoning / toolCall items) so the output is
// well-formed regardless of how the underlying deltas interleave.
//
// State machine:
//
//	runs.TurnStart       → segment.started
//	runs.MessageDelta    → close reasoning + item.started(agentMessage,lazy) + item.delta(content)
//	runs.ReasoningDelta  → close text + item.started(reasoning,lazy) + item.delta(reasoning)
//	runs.ToolCallStart   → close text+reasoning + item.started(toolCall) + item.delta(toolArguments)
//	runs.ToolCallEnd     → item.completed(toolCall)
//	runs.TurnEnd         → close open items + segment.finished(outcome)
//	runs.TurnInterrupted → close open items + interrupt Item(s) + segment.finished(outcome:interrupt)
//	runs.ErrorEvent      → captured, surfaced in segment.finished(outcome:error)
//	runs.CompactBoundary → compaction Item (item.started + item.completed)
//
// MemoryUpdated is not surfaced here: extracted long-term memory is internal
// housekeeping with no client-facing surface (nothing folds a memory event).
type translator struct {
	runID     string // the STABLE logical run → RunRef.id + every Item.runId
	sessionID string
	// segmentID is THIS streamed segment. Item ids derive from it (not runID) so a
	// run's resume segments — which share runID — never collide on item ids.
	segmentID string
	provider  string // run's provider → RunRef.provider on segment.started
	model     string // run's model → RunRef.model on segment.started
	createdAt time.Time
	resume    *resumeBinding
	itemSeq   int
	step      int // tool-call ordinal, surfaced as segment.progress.step (API.md §5)

	// userInput is the run's opening user message, emitted as the first
	// Item (userMessage) right after segment.started. Set only for root runs
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
	createdAt   time.Time
	name        string
	args        string // raw JSON arguments, replayed to rebuild the invocation at completion
	safetyClass string // wire SafetyClass, carried so item.completed matches item.started
}

func newTranslator(sessionID, runID, segmentID string, userInput []protocol.ContentBlock, resume *resumeBinding, provider, model string) *translator {
	return &translator{
		runID:     runID,
		sessionID: sessionID,
		segmentID: segmentID,
		provider:  provider,
		model:     model,
		resume:    resume,
		userInput: userInput,
		tools:     map[string]*openTool{},
	}
}

func (t *translator) nextItemID() string {
	t.itemSeq++
	return protocol.IDPrefixItem + t.segmentID + "_" + strconv.Itoa(t.itemSeq)
}

// userMessageItemID is the deterministic id of a segment's opening userMessage
// Item — derived from the segmentId so [StartRun] can return it in the response
// (for optimistic-bubble reconciliation) and the translator stamps the same one
// onto the streamed + persisted Item. Segment-scoped (not run-scoped) so a run's
// resume segments don't collide. The "_u" suffix keeps it clear of the
// translator's numbered items (_1, _2, …).
func userMessageItemID(segmentID string) string {
	return protocol.IDPrefixItem + segmentID + "_u"
}

// open is the first thing emitted on EVERY run segment — a run's opening one and
// its resume continuations alike. It guarantees segment.started leads the stream (the
// client's segment boundary), then the opening userMessage Item and, for a
// resumed segment, the terminal item.completed for any question the parked run
// left open. Driven by pumpRun before any turn event, so it never depends on a
// turn-level TurnStart (which continuations don't emit).
func (t *translator) open() []protocol.StreamEvent {
	createdAt := t.createdAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	out := []protocol.StreamEvent{{
		Type: protocol.StreamSegmentStarted,
		Run: &protocol.RunRef{
			ID:        t.runID,
			SessionID: t.sessionID,
			Provider:  t.provider,
			Model:     t.model,
			Status:    protocol.RunStatusRunning,
			CreatedAt: createdAt,
		},
	}}
	out = append(out, t.openUserMessage()...)
	return append(out, t.resumeQuestionCompletions()...)
}

// translate maps one Lyra turn event to zero or more StreamEvents.
func (t *translator) translate(ev runs.EngineEvent) []protocol.StreamEvent {
	switch e := ev.(type) {
	case runs.TurnStart:
		// segment.started is emitted by open() at the start of every run segment
		// (so continuation runs get it too — they carry no turn.TurnStart),
		// not here. Nothing to do for the turn-level TurnStart.
		return nil
	case runs.MessageDelta:
		// Close any open reasoning before emitting text — reasoning and
		// text cannot be concurrently open (API.md §5: at most one
		// streaming item at a time).
		out := t.closeReasoning()
		return append(out, t.appendText(e.Text)...)
	case runs.ReasoningDelta:
		// Close any open text before emitting reasoning.
		out := t.closeText()
		return append(out, t.appendReasoning(e.Text)...)
	case runs.ToolCallStart:
		return t.toolStart(e)
	case runs.ToolCallEnd:
		return t.toolEnd(e)
	case runs.UsageReported:
		return t.usageProgress(e)
	case runs.SteerMessage:
		return t.steerMessage(e)
	case runs.TodosUpdated:
		return t.todosSnapshot(e)
	case runs.ErrorEvent:
		t.errMsg = e.Message
		t.errCode = e.Code
		return nil
	case runs.CompactBoundary:
		return t.compaction(e)
	case runs.MemoryUpdated:
		return nil
	case runs.TurnInterrupted:
		return t.interrupt(e)
	case runs.TurnEnd:
		return t.turnEnd(e)
	default:
		panic("server: unhandled application engine event")
	}
}
