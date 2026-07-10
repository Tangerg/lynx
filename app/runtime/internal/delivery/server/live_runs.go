package server

// isRunLive reports whether a run is currently being pumped in this process.
func (s *Server) isRunLive(runID string) bool {
	return s.coordinator.Contains(runID)
}
