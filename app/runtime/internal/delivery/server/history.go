package server

import (
	"encoding/json"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/runsegment"
)

// sideEffectEvent converts one wire StreamEvent into the delivery-neutral
// payload that kernel/runsegment executes. The wire blob encoding stays here:
// transcript history is the replay store for the protocol-facing UI timeline.
func (s *Server) sideEffectEvent(runID, sessionID, parentRunID, cwd string, se protocol.StreamEvent, provider, model string) runsegment.Event {
	var out runsegment.Event
	switch se.Type {
	case protocol.StreamItemCompleted:
		out.Item = transcriptItem(sessionID, se.Item)
		paths := toolFileChangedPaths(se)
		if len(paths) > 0 {
			out.FilesChanged = &runsegment.FilesChanged{Cwd: cwd, Paths: paths}
		}
	case protocol.StreamRunStarted:
		out.Run = transcriptRun(sessionID, se.Run, false)
	case protocol.StreamRunFinished:
		// run.finished carries only the outcome; synthesize the terminal
		// RunRef so history records the run's final status + outcome. The
		// message log is now in its terminal post-maintenance (post-compaction)
		// shape — runsegment resolves MessageCount as this run's watermark, the
		// boundary sessions.rollback / fork{fromRunId} truncate to (B4).
		out.Run = transcriptRun(sessionID, &protocol.RunRef{
			ID:          runID,
			SessionID:   sessionID,
			ParentRunID: parentRunID,
			Provider:    provider,
			Model:       model,
			Status:      protocol.RunStatusFinished,
			Outcome:     se.Outcome,
			// Carry the run's start time forward: the terminal RunRef replaces the
			// whole stored blob, so without this CreatedAt persists as zero and the
			// rollback/fork boundary math (+ runs.list) loses the run's timeline key.
			CreatedAt:  s.runCreatedAt(runID),
			FinishedAt: time.Now().UTC(),
		}, true)
		// NOTE: the file-checkpoint snapshot is deliberately NOT taken here.
		// It used to run synchronously on this run.finished path, ahead of the
		// hub append — so a slow `git add` (e.g. a session opened on a huge
		// dir) blocked the terminal event from reaching the client AND blocked
		// the pump's teardown, leaving the run stuck "running" forever. It now
		// runs asynchronously off the critical path in pumpRun's teardown.
	}
	return out
}

func transcriptItem(sessionID string, item *protocol.Item) *transcript.Item {
	if item == nil {
		return nil
	}
	blob, err := json.Marshal(item)
	if err != nil {
		return nil
	}
	return &transcript.Item{
		SessionID: sessionID,
		RunID:     item.RunID,
		ItemID:    item.ID,
		CreatedAt: item.CreatedAt,
		Blob:      blob,
	}
}

func transcriptRun(sessionID string, run *protocol.RunRef, terminal bool) *runsegment.RunRecord {
	if run == nil {
		return nil
	}
	blob, err := json.Marshal(run)
	if err != nil {
		return nil
	}
	mark := -1
	return &runsegment.RunRecord{Run: transcript.Run{
		SessionID: sessionID,
		RunID:     run.ID,
		UpdatedAt: time.Now().UTC(),
		Blob:      blob,
		Mark:      mark,
	}, Terminal: terminal}
}
