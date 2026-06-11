package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/service/history"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// B4 turn-granular checkpoints (AUX_API §4): sessions.rollback truncates a
// session's history at a run boundary in place; sessions.fork{fromRunId}
// truncate-copies it into a child. Both reason over the per-run message
// watermark (history.Run.Mark, recorded at run.finished — see history.go) to
// map a run boundary onto a chat-memory message count, since the message log
// itself carries no run markers.

// runRecord pairs a parsed wire RunRef with its persisted message watermark.
type runRecord struct {
	ref  protocol.RunRef
	mark int
}

// isRootRun reports whether r opens a turn (a runs.start run) rather than a
// continuation (runs.resume → ParentRunID) or a subagent (SpawnedByItemID).
func isRootRun(r protocol.RunRef) bool {
	return r.ParentRunID == "" && r.SpawnedByItemID == ""
}

// runTimeline parses a session's persisted runs into records ordered by
// CreatedAt — the wall-clock turn order rollback / fork reason about.
func runTimeline(runs []history.Run) ([]runRecord, error) {
	out := make([]runRecord, 0, len(runs))
	for _, r := range runs {
		var ref protocol.RunRef
		if err := json.Unmarshal(r.Blob, &ref); err != nil {
			return nil, fmt.Errorf("server: decode run %q: %w", r.RunID, err)
		}
		out = append(out, runRecord{ref: ref, mark: r.Mark})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ref.CreatedAt.Before(out[j].ref.CreatedAt)
	})
	return out, nil
}

// boundaryAt computes the inclusive-keep split of a run timeline at runID:
//
//   - keepMark: the message watermark to keep — the Mark of runID's chain
//     terminal (the last run before the first ROOT run after runID), so runID's
//     own continuation chain is kept. -1 when that run's watermark is unknown
//     (in-flight / pre-watermark), which the caller clamps.
//   - dropped: the runs at/after the boundary, in timeline order (runID's
//     continuation chain stays kept; the next root run + everything after it,
//     including subagent runs, is dropped).
//   - boundaryTime: the first dropped root run's CreatedAt — the cut-off used
//     to attribute subagent child sessions to dropped runs. Zero when nothing
//     is dropped, or when runID is empty (drop everything).
//
// runID=="" drops every run (keepMark 0). requireRoot rejects a non-root runID
// with invalid_params (rollback addresses root runs only; fork is lax).
func boundaryAt(timeline []runRecord, runID string, requireRoot bool) (keepMark int, dropped []runRecord, boundaryTime time.Time, err error) {
	if runID == "" {
		return 0, append([]runRecord(nil), timeline...), time.Time{}, nil
	}
	idx := -1
	for i, rec := range timeline {
		if rec.ref.ID == runID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, nil, time.Time{}, protocol.ErrRunNotFound
	}
	if requireRoot && !isRootRun(timeline[idx].ref) {
		return 0, nil, time.Time{}, fmt.Errorf("%w: run %q is not a root run", protocol.ErrInvalidParams, runID)
	}
	for k := idx + 1; k < len(timeline); k++ {
		if isRootRun(timeline[k].ref) {
			return timeline[k-1].mark, append([]runRecord(nil), timeline[k:]...), timeline[k].ref.CreatedAt, nil
		}
	}
	// No root run after runID — its turn (incl. continuations) is the latest,
	// so there is nothing to drop / everything up to it is copied.
	return timeline[len(timeline)-1].mark, nil, time.Time{}, nil
}

// hasActiveRun reports whether the session has a run in flight — the
// session_busy guard: rolling back under a live run would race its history
// append (AUX_API §4.1).
func (s *Server) hasActiveRun(sessionID string) bool {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	for _, e := range s.runs {
		if e.sessionID == sessionID {
			return true
		}
	}
	return false
}

// RollbackSession discards the runs after the kept boundary, truncating the
// session in place at a run granularity (AUX_API §4.1). Destructive: it
// truncates the chat-memory log to the kept watermark, deletes the dropped
// runs' durable items + records, clears their dangling interrupts, and purges
// the subagent child sessions they spawned. ToRunID is inclusive-keep (omit =
// clear to empty). Rejected with session_busy while a run is in flight.
func (s *Server) RollbackSession(ctx context.Context, in protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error) {
	ses, err := s.rt.Session().Get(ctx, in.SessionID)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	if s.hasActiveRun(in.SessionID) {
		return nil, fmt.Errorf("%w: session %q has a run in flight", protocol.ErrSessionBusy, in.SessionID)
	}

	items, runs, err := s.rt.History().List(ctx, in.SessionID)
	if err != nil {
		return nil, err
	}
	timeline, err := runTimeline(runs)
	if err != nil {
		return nil, err
	}
	keepMark, dropped, boundaryTime, err := boundaryAt(timeline, in.ToRunID, true)
	if err != nil {
		return nil, err
	}
	if len(dropped) == 0 {
		// ToRunID is already the latest turn — nothing after it to drop.
		out := s.sessionToWire(ses)
		return &protocol.RollbackSessionResponse{Session: &out, DroppedRuns: []protocol.DroppedRun{}}, nil
	}

	// Truncate the chat-memory log to the kept watermark. An unknown (-1) mark
	// (chain terminal still in-flight / pre-watermark) clamps to the current
	// count — keep what's there rather than guess at a boundary we never recorded.
	if keepMark >= 0 {
		if err := s.rt.TruncateMessages(ctx, in.SessionID, keepMark); err != nil {
			return nil, err
		}
	}

	// Drop each run's durable items + run record, and clear any open interrupt
	// dangling on it (rollback over a parked run un-parks it).
	for _, rec := range dropped {
		_ = s.rt.History().DeleteRun(ctx, in.SessionID, rec.ref.ID)
		_ = s.rt.Interrupts().Delete(ctx, rec.ref.ID)
	}

	// Purge the subagent child sessions the dropped runs spawned (whole subtree).
	// Attribution is by spawn time: a subtask of a kept run started before the
	// drop boundary, one of a dropped run at/after it. This is exact because a
	// session runs its turns sequentially and rollback requires it idle (the
	// session_busy guard above), so run windows don't overlap.
	s.purgeSubtasksAfter(ctx, in.SessionID, boundaryTime)

	// Each dropped run reports its opening user input so the client can
	// re-populate the composer.
	userByRun := openingUserInput(items)
	out := make([]protocol.DroppedRun, 0, len(dropped))
	for _, rec := range dropped {
		out = append(out, protocol.DroppedRun{Run: rec.ref, UserInput: userByRun[rec.ref.ID]})
	}
	sess := s.sessionToWire(ses)
	return &protocol.RollbackSessionResponse{Session: &sess, DroppedRuns: out}, nil
}

// openingUserInput maps each run id to the content of its FIRST userMessage
// item — the opening turn the client re-populates the composer from. Runs with
// no opening user turn (resume / edit continuations) are absent from the map.
func openingUserInput(items []history.Item) map[string][]protocol.ContentBlock {
	out := map[string][]protocol.ContentBlock{}
	for _, it := range items {
		if _, seen := out[it.RunID]; seen {
			continue
		}
		var item protocol.Item
		if err := json.Unmarshal(it.Blob, &item); err != nil {
			continue
		}
		if item.Type != protocol.ItemTypeUserMessage {
			continue
		}
		out[it.RunID] = item.Content
	}
	return out
}

// purgeSubtasksAfter purges the subagent child sessions of parentID that were
// spawned at/after boundary (a zero boundary purges all children — the drop-all
// rollback). See RollbackSession for why spawn time is exact attribution.
func (s *Server) purgeSubtasksAfter(ctx context.Context, parentID string, boundary time.Time) {
	children, err := s.rt.Session().Children(ctx, parentID)
	if err != nil {
		return
	}
	for _, child := range children {
		if !boundary.IsZero() && child.StartedAt.Before(boundary) {
			continue
		}
		s.purgeSession(ctx, child.ID)
	}
}

// purgeSession deletes a session and its whole descendant subtree depth-first:
// chat-memory messages, durable history (items + runs), and the session row.
// Best-effort — a partial failure still removes the leaves it reached.
func (s *Server) purgeSession(ctx context.Context, sessionID string) {
	if children, err := s.rt.Session().Children(ctx, sessionID); err == nil {
		for _, c := range children {
			s.purgeSession(ctx, c.ID)
		}
	}
	_ = s.rt.TruncateMessages(ctx, sessionID, 0) // clear chat-memory
	_ = s.rt.History().DeleteSession(ctx, sessionID)
	_ = s.rt.Session().Delete(ctx, sessionID)
}
