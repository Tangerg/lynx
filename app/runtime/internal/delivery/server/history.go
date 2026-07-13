package server

import (
	"encoding/json"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// sideEffectEvent derives one wire StreamEvent's durable transcript projections
// (an item, a run record) plus its non-durable workspace nudge. The run-state
// transition and the open-interrupt record are added by the projector, which
// knows the event's terminal/interrupt shape. The wire blob encoding stays here:
// transcript history is the replay store for the protocol-facing UI timeline.
// createdAt is the run's start time (captured at segment open), carried onto the
// synthesized terminal RunRef so the persisted run keeps its timeline key.
func sideEffectEvent(runID, sessionID, cwd string, se protocol.StreamEvent, provider, model string, createdAt time.Time) (execution.EventCommit, *runs.Nudge) {
	commit := execution.EventCommit{SessionID: sessionID}
	var nudge *runs.Nudge
	switch se.Type {
	case protocol.StreamItemCompleted:
		commit.Item = transcriptItem(sessionID, se.Item)
		if paths := toolFileChangedPaths(se); len(paths) > 0 {
			nudge = &runs.Nudge{Cwd: cwd, Paths: paths}
		}
	case protocol.StreamSegmentStarted:
		commit.Run = transcriptRun(sessionID, se.Run)
	case protocol.StreamSegmentFinished:
		// segment.finished carries only the outcome; synthesize the terminal RunRef so
		// history records the run's final status + outcome. The message log is now
		// in its terminal post-maintenance (post-compaction) shape — the committer
		// resolves the terminal watermark (Mark) as this run's boundary, the mark
		// sessions.rollback / fork{fromRunId} truncate to (B4).
		commit.Run = transcriptRun(sessionID, &protocol.RunRef{
			ID:        runID,
			SessionID: sessionID,
			Provider:  provider,
			Model:     model,
			Status:    protocol.RunStatusFinished,
			Outcome:   se.Outcome,
			// Carry the run's start time forward: the terminal RunRef replaces the
			// whole stored blob, so without this CreatedAt persists as zero and the
			// rollback/fork boundary math (+ runs.list) loses the run's timeline key.
			CreatedAt:  createdAt,
			FinishedAt: time.Now().UTC(),
		})
		// NOTE: the file-checkpoint snapshot is deliberately NOT taken here. It
		// runs asynchronously off the critical path in the pump's teardown Finish,
		// so a slow `git add` can't block the terminal event or the teardown.
	}
	return commit, nudge
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

// transcriptRun marshals a run record for persistence, with Mark left unresolved
// (-1): the committer fills the terminal watermark inside the commit for a
// terminalizing run and leaves it -1 (unknown) for the opening segment.started.
func transcriptRun(sessionID string, run *protocol.RunRef) *transcript.Run {
	if run == nil {
		return nil
	}
	blob, err := json.Marshal(run)
	if err != nil {
		return nil
	}
	return &transcript.Run{
		SessionID: sessionID,
		RunID:     run.ID,
		UpdatedAt: time.Now().UTC(),
		Blob:      blob,
		Mark:      -1,
	}
}
