package server

import (
	"context"
	"encoding/json"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// persistStreamEvent records the durable side of one stream event to the
// Item-history store as a run streams (API.md §7.4 / §10.3): completed
// Items are appended; the run's RunRef is upserted on start (running) and
// finish (finished + outcome). Best-effort — persistence errors never fail
// the live stream. The nil guard only covers test stubs; the real runtime
// always supplies a history store.
func (s *Server) persistStreamEvent(ctx context.Context, runID, sessionID, parentRunID string, se protocol.StreamEvent, model string) {
	if s.rt.Transcript() == nil {
		return
	}
	switch se.Type {
	case protocol.StreamItemCompleted:
		s.persistItem(ctx, sessionID, se.Item)
	case protocol.StreamRunStarted:
		// Start: status running, watermark unknown (-1) until finish.
		s.persistRun(ctx, sessionID, se.Run, -1)
	case protocol.StreamRunFinished:
		// run.finished carries only the outcome; synthesize the terminal
		// RunRef so history records the run's final status + outcome. The
		// message log is now in its terminal post-maintenance (post-compaction)
		// shape — the chat service emits run.finished only after flushSteering
		// + postTurnMaintenance — so MessageCount here is this run's watermark,
		// the boundary sessions.rollback / fork{fromRunId} truncate to (B4).
		mark, err := s.rt.MessageCount(ctx, sessionID)
		if err != nil {
			mark = -1
		}
		s.persistRun(ctx, sessionID, &protocol.RunRef{
			ID:          runID,
			SessionID:   sessionID,
			ParentRunID: parentRunID,
			Model:       model,
			Status:      protocol.RunStatusFinished,
			Outcome:     se.Outcome,
			FinishedAt:  time.Now().UTC(),
		}, mark)
		// NOTE: the file-checkpoint snapshot is deliberately NOT taken here.
		// It used to run synchronously on this run.finished path, ahead of the
		// hub append — so a slow `git add` (e.g. a session opened on a huge
		// dir) blocked the terminal event from reaching the client AND blocked
		// the pump's teardown, leaving the run stuck "running" forever. It now
		// runs asynchronously off the critical path in pumpRun's teardown.
	}
}

// persistItem appends one completed Item to the history store.
func (s *Server) persistItem(ctx context.Context, sessionID string, item *protocol.Item) {
	store := s.rt.Transcript()
	if store == nil || item == nil {
		return
	}
	blob, err := json.Marshal(item)
	if err != nil {
		return
	}
	if err := store.AppendItem(ctx, transcript.Item{
		SessionID: sessionID,
		RunID:     item.RunID,
		ItemID:    item.ID,
		CreatedAt: item.CreatedAt,
		Blob:      blob,
	}); err != nil {
		// Best-effort: a dropped item never fails the live stream, but record it
		// on the run's trace span (ctx keeps the span, full-link) so the loss is
		// observable instead of silent — items.list would otherwise just be
		// short a record with no signal why.
		trace.SpanFromContext(ctx).RecordError(err)
	}
}

// persistRun upserts one RunRef into the history store. mark is the per-run
// message watermark recorded at finish (-1 at start / when unknown).
func (s *Server) persistRun(ctx context.Context, sessionID string, run *protocol.RunRef, mark int) {
	store := s.rt.Transcript()
	if store == nil || run == nil {
		return
	}
	blob, err := json.Marshal(run)
	if err != nil {
		return
	}
	if err := store.PutRun(ctx, transcript.Run{
		SessionID: sessionID,
		RunID:     run.ID,
		UpdatedAt: time.Now().UTC(),
		Blob:      blob,
		Mark:      mark,
	}); err != nil {
		// Best-effort like persistItem: don't fail the stream, but surface the
		// loss on the span — a missing RunRef means runs.subscribe can't replay
		// this run, which is worth a trace signal rather than silence.
		trace.SpanFromContext(ctx).RecordError(err)
	}
}
