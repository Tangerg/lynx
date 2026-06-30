package server

import "time"

// hasActiveRun reports whether the session has a run in flight — the
// session_busy guard: rolling back under a live run would race its history
// append (AUX_API §4.1). Includes an in-progress admission (claimed but not yet
// registered) so a rollback can't slip in alongside a starting run.
func (s *Server) hasActiveRun(sessionID string) bool {
	return s.runs.ActiveSession(sessionID)
}

// claimSession atomically reserves the session's single-writer slot for an
// admitting run, returning false when the session already has a run in flight
// or another admission in progress. It closes the TOCTOU gap between the busy
// check and the run's registration in s.runs (openSegment): the registry checks
// and reserves atomically. Pair every true return with a releaseSession
// (deferred), which is safe to run after openSegment has registered the run.
func (s *Server) claimSession(sessionID string) bool {
	return s.runs.ClaimSession(sessionID)
}

// releaseSession drops a claimSession reservation.
func (s *Server) releaseSession(sessionID string) {
	s.runs.ReleaseSession(sessionID)
}

// hasActiveRunSharingCwd returns the id of an in-flight run's session whose
// canonical working tree is cwd, or "" when none. The broader busy guard a file
// restore needs: its `git reset --hard` WRITES the working tree, which a sibling
// session sharing the cwd would race (a fork inherits its parent's cwd; two
// sessions can open one dir) — and that sibling's tool writes never go through
// the checkpoint lock. An empty cwd matches nothing (a session with no cwd has
// no checkpoint tree). cwd must already be canonical.
func (s *Server) hasActiveRunSharingCwd(cwd string) string {
	return s.runs.ActiveSessionWithCwd(cwd)
}

// isRunLive reports whether a run is currently being pumped in this process.
func (s *Server) isRunLive(runID string) bool {
	return s.runs.Contains(runID)
}

// cancelReasonFor returns the runs.cancel reason recorded for a run, or ""
// when it wasn't canceled with one.
func (s *Server) cancelReasonFor(runID string) string {
	return s.runs.CancelReason(runID)
}

// runCreatedAt returns the run's start time (segment open). The terminal RunRef
// carries it as CreatedAt so the persisted run keeps its authoritative timeline
// key — the finish event has no start time of its own, and the synthesized
// terminal RunRef replaces the whole stored blob (PutRun upsert), so omitting
// it would zero CreatedAt for every consumer (runs.list + the rollback/fork
// boundary math, which then over-purges). The active record is still live at
// finish: emit (and this persist) run before the pump's teardown deletes it.
func (s *Server) runCreatedAt(runID string) time.Time {
	if e, ok := s.runs.Get(runID); ok {
		return e.Record.CreatedAt
	}
	return time.Time{}
}
