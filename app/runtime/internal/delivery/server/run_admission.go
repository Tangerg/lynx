package server

// hasActiveRun reports whether the session has a run in flight — the
// session_busy guard: rolling back under a live run would race its history
// append (AUX_API §4.1). Includes an in-progress admission (claimed but not yet
// registered) so a rollback can't slip in alongside a starting run.
func (s *Server) hasActiveRun(sessionID string) bool {
	return s.coordinator.ActiveSession(sessionID)
}
