package server

// dropCheckpoints removes a session's shadow repo (on session delete). Working-
// tree restore now rides the sessions coordinator's checkpoint restorer
// ([sessions.Coordinator.RollbackFiles]); only the drop stays a delivery concern.
func (s *Server) dropCheckpoints(sessionID string) {
	_ = s.checkpoints.DropSession(sessionID)
}
