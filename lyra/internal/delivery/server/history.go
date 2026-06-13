package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/delivery/protocol"
	"github.com/Tangerg/lynx/lyra/internal/domain/transcript"
)

// persistStreamEvent records the durable side of one stream event to the
// Item-history store as a run streams (API.md §7.4 / §10.3): completed
// Items are appended; the run's RunRef is upserted on start (running) and
// finish (finished + outcome). Best-effort — persistence errors never fail
// the live stream. The nil guard only covers test stubs; the real runtime
// always supplies a history store.
func (s *Server) persistStreamEvent(ctx context.Context, runID, sessionID, parentRunID string, se protocol.StreamEvent) {
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
			Status:      protocol.RunStatusFinished,
			Outcome:     se.Outcome,
			FinishedAt:  time.Now().UTC(),
		}, mark)
		// Anchor a file snapshot at this run boundary so a later
		// rollback{restoreType:files|both} can restore the working tree here.
		s.snapshotCheckpoint(ctx, sessionID, runID)
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
	_ = store.AppendItem(ctx, transcript.Item{
		SessionID: sessionID,
		RunID:     item.RunID,
		ItemID:    item.ID,
		CreatedAt: item.CreatedAt,
		Blob:      blob,
	})
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
	_ = store.PutRun(ctx, transcript.Run{
		SessionID: sessionID,
		RunID:     run.ID,
		UpdatedAt: time.Now().UTC(),
		Blob:      blob,
		Mark:      mark,
	})
}
