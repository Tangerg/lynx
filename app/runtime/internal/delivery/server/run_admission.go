package server

// The session-busy guards, delegated to the run Coordinator (which owns the
// single-writer-per-session admission slot). These thin methods keep the guard
// vocabulary at the delivery call sites (rollback / mutation) unchanged.

// hasActiveRun reports whether the session has a run in flight — the
// session_busy guard: rolling back under a live run would race its history
// append (AUX_API §4.1). Includes an in-progress admission (claimed but not yet
// registered) so a rollback can't slip in alongside a starting run.
func (s *Server) hasActiveRun(sessionID string) bool {
	return s.coordinator.ActiveSession(sessionID)
}

// claimSession atomically reserves the session's single-writer slot for an
// admitting run, returning false when the session already has a run in flight or
// another admission in progress. Pair every true return with a releaseSession.
func (s *Server) claimSession(sessionID string) bool {
	return s.coordinator.ClaimSession(sessionID)
}

// releaseSession drops a claimSession reservation.
func (s *Server) releaseSession(sessionID string) {
	s.coordinator.ReleaseSession(sessionID)
}
