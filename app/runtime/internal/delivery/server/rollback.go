package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/fspath"
)

// sessions.rollback truncates a
// session's history at a run boundary in place; sessions.fork{fromRunId}
// truncate-copies it into a child. Both reason over the per-run message
// watermark (transcript.Run.Mark, recorded at run.finished — see transcript.go) to
// map a run boundary onto a chat-memory message count, since the message log
// itself carries no run markers.

// runNodes lifts the structured timeline fields out of each persisted run's
// opaque wire blob (a marshaled [protocol.RunRef]) so the domain boundary math
// ([transcript.BoundaryAt]) stays wire-free. It also returns a by-id index of
// the original RunRefs, because the rollback response reports dropped runs as
// full wire RunRefs.
func runNodes(runs []transcript.Run) ([]transcript.RunNode, map[string]protocol.RunRef, error) {
	nodes := make([]transcript.RunNode, 0, len(runs))
	byID := make(map[string]protocol.RunRef, len(runs))
	for _, r := range runs {
		var ref protocol.RunRef
		if err := json.Unmarshal(r.Blob, &ref); err != nil {
			return nil, nil, fmt.Errorf("server: decode run %q: %w", r.RunID, err)
		}
		nodes = append(nodes, transcript.RunNode{
			ID:              ref.ID,
			ParentRunID:     ref.ParentRunID,
			SpawnedByItemID: ref.SpawnedByItemID,
			CreatedAt:       ref.CreatedAt,
			Mark:            r.Mark,
		})
		byID[ref.ID] = ref
	}
	return nodes, byID, nil
}

// wireBoundaryErr maps the transcript boundary sentinels onto their wire errors
// (the domain layer is protocol-free; the adapter owns the wire mapping).
func wireBoundaryErr(err error) error {
	switch {
	case errors.Is(err, transcript.ErrRunNotFound):
		return protocol.ErrRunNotFound
	case errors.Is(err, transcript.ErrNotRoot):
		return fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
	default:
		return err
	}
}

// hasActiveRun reports whether the session has a run in flight — the
// session_busy guard: rolling back under a live run would race its history
// append (AUX_API §4.1). Includes an in-progress admission (claimed but not yet
// registered) so a rollback can't slip in alongside a starting run.
func (s *Server) hasActiveRun(sessionID string) bool {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	return s.activeForSessionLocked(sessionID)
}

// activeForSessionLocked reports whether the session has a run in flight OR an
// admission in progress (a runs.start / runs.resume that claimed the session
// but hasn't registered its run yet). Caller holds runMu.
func (s *Server) activeForSessionLocked(sessionID string) bool {
	if _, ok := s.claiming[sessionID]; ok {
		return true
	}
	for _, e := range s.runs {
		if e.sessionID == sessionID {
			return true
		}
	}
	return false
}

// claimSession atomically reserves the session's single-writer slot for an
// admitting run, returning false when the session already has a run in flight
// or another admission in progress. It closes the TOCTOU gap between the busy
// check and the run's registration in s.runs (openSegment): under one runMu
// hold it both checks and reserves. Pair every true return with a
// releaseSession (deferred), which is safe to run after openSegment has
// registered the run — at that point s.runs marks the session active, so there
// is never an instant where neither the claim nor s.runs holds it.
func (s *Server) claimSession(sessionID string) bool {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.activeForSessionLocked(sessionID) {
		return false
	}
	if s.claiming == nil { // a Server built as a bare literal (tests) — keep the zero value useful
		s.claiming = map[string]struct{}{}
	}
	s.claiming[sessionID] = struct{}{}
	return true
}

// releaseSession drops a claimSession reservation.
func (s *Server) releaseSession(sessionID string) {
	s.runMu.Lock()
	delete(s.claiming, sessionID)
	s.runMu.Unlock()
}

// hasActiveRunSharingCwd returns the id of an in-flight run's session whose
// canonical working tree is cwd, or "" when none. The broader busy guard a file
// restore needs: its `git reset --hard` WRITES the working tree, which a sibling
// session sharing the cwd would race (a fork inherits its parent's cwd; two
// sessions can open one dir) — and that sibling's tool writes never go through
// the checkpoint lock. An empty cwd matches nothing (a session with no cwd has
// no checkpoint tree). cwd must already be canonical (runEntry.cwd is).
func (s *Server) hasActiveRunSharingCwd(cwd string) string {
	if cwd == "" {
		return ""
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	for _, e := range s.runs {
		if e.cwd == cwd {
			return e.sessionID
		}
	}
	return ""
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

	restoreType := in.RestoreType
	if restoreType == "" {
		restoreType = protocol.RestoreHistory
	}
	switch restoreType {
	case protocol.RestoreFiles, protocol.RestoreHistory, protocol.RestoreBoth:
	default:
		// An unknown restoreType must be rejected, not silently no-op'd into a
		// success that restores nothing.
		return nil, fmt.Errorf("%w: unknown restoreType %q", protocol.ErrInvalidParams, restoreType)
	}
	doFiles := restoreType == protocol.RestoreFiles || restoreType == protocol.RestoreBoth
	doHistory := restoreType == protocol.RestoreHistory || restoreType == protocol.RestoreBoth
	if doFiles && in.ToRunID == "" {
		return nil, fmt.Errorf("%w: restoreType %q requires toRunId", protocol.ErrInvalidParams, restoreType)
	}

	// A file restore's `git reset --hard` writes the working tree, which a sibling
	// session sharing this cwd (a fork inherits the parent's cwd; two sessions can
	// open one dir) would race — and that sibling's tool writes never take the
	// checkpoint lock. The per-session guard above only covers THIS session, so
	// widen it to the whole tree for file restores. (History-only rollback touches
	// just this session's log, so the per-session guard suffices.)
	if doFiles {
		if busy := s.hasActiveRunSharingCwd(fspath.Canonical(ses.Cwd)); busy != "" {
			return nil, fmt.Errorf("%w: session %q shares this working tree and has a run in flight", protocol.ErrSessionBusy, busy)
		}
	}

	items, runs, err := s.rt.Transcript().List(ctx, in.SessionID)
	if err != nil {
		return nil, err
	}
	nodes, refByID, err := runNodes(runs)
	if err != nil {
		return nil, err
	}
	b, err := transcript.BoundaryAt(nodes, in.ToRunID, true)
	if err != nil {
		return nil, wireBoundaryErr(err)
	}

	// Files first — for "both" this is the atomicity guarantee: if the working
	// tree can't be restored, return now and leave history untouched.
	if doFiles {
		if err := s.restoreCheckpoint(ctx, in.SessionID, in.ToRunID); err != nil {
			return nil, err
		}
	}

	if !doHistory || len(b.Dropped) == 0 {
		// History stays (files-only rollback), or ToRunID is already the latest
		// turn so there's nothing after it to drop.
		out := s.sessionToWire(ses, s.liveStatus(ctx, ses.ID))
		return &protocol.RollbackSessionResponse{Session: &out, DroppedRuns: []protocol.DroppedRun{}}, nil
	}

	// Truncate the chat-memory log to the kept watermark. An unknown (-1) mark
	// (chain terminal still in-flight / pre-watermark) clamps to the current
	// count — keep what's there rather than guess at a boundary we never recorded.
	if b.KeepMark >= 0 {
		if err := s.rt.TruncateMessages(ctx, in.SessionID, b.KeepMark); err != nil {
			return nil, err
		}
	}

	// Drop each run's durable items + run record, and clear any open interrupt
	// dangling on it (rollback over a parked run un-parks it).
	for _, rec := range b.Dropped {
		_ = s.rt.Transcript().DeleteRun(ctx, in.SessionID, rec.ID)
		_ = s.rt.Interrupts().Delete(ctx, rec.ID)
	}

	// Purge the subagent child sessions the dropped runs spawned (whole subtree).
	// Attribution is by spawn time: a subtask of a kept run started before the
	// drop boundary, one of a dropped run at/after it. This is exact because a
	// session runs its turns sequentially and rollback requires it idle (the
	// session_busy guard above), so run windows don't overlap.
	s.purgeSubtasksAfter(ctx, in.SessionID, b.BoundaryTime)

	// Each dropped run reports its opening user input so the client can
	// re-populate the composer.
	userByRun := openingUserInput(items)
	out := make([]protocol.DroppedRun, 0, len(b.Dropped))
	for _, rec := range b.Dropped {
		out = append(out, protocol.DroppedRun{Run: refByID[rec.ID], UserInput: userByRun[rec.ID]})
	}
	sess := s.sessionToWire(ses, s.liveStatus(ctx, ses.ID))
	return &protocol.RollbackSessionResponse{Session: &sess, DroppedRuns: out}, nil
}

// openingUserInput maps each run id to the content of its FIRST userMessage
// item — the opening turn the client re-populates the composer from. Runs with
// no opening user turn (resume / edit continuations) are absent from the map.
func openingUserInput(items []transcript.Item) map[string][]protocol.ContentBlock {
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
	_ = s.rt.Transcript().DeleteSession(ctx, sessionID)
	_ = s.rt.Session().Delete(ctx, sessionID)
	s.rt.Chat().ForgetSession(sessionID) // process-local SessionStart gate
}
