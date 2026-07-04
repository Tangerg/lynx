package server

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

type sessionClaimer struct {
	s *Server
}

func (c sessionClaimer) ClaimSession(sessionID string) bool {
	return c.s.claimSession(sessionID)
}

func (c sessionClaimer) ReleaseSession(sessionID string) {
	c.s.releaseSession(sessionID)
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
