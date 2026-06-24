package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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

	// The boundary is decided (wire-coupled: it decodes the stored RunRefs); the
	// destructive write-set is the coordinator's. It truncates the chat-memory
	// log to the kept watermark + drops each dropped run's items/record + dangling
	// interrupt as ONE transaction (a failure can't leave a run whose messages
	// were already truncated away), then purges the subagent subtree those runs
	// spawned. A -1 KeepMark leaves the log untouched.
	dropIDs := make([]string, len(b.Dropped))
	for i, rec := range b.Dropped {
		dropIDs[i] = rec.ID
	}
	if err := s.coordinator().Rollback(ctx, in.SessionID, b.KeepMark, dropIDs, b.BoundaryTime); err != nil {
		return nil, err
	}

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
