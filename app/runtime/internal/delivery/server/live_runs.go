package server

import "time"

// isRunLive reports whether a run is currently being pumped in this process.
func (s *Server) isRunLive(runID string) bool {
	return s.runs.Contains(runID)
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
