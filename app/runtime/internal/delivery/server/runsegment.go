package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/runsegment"
)

// The durable side-effect coordinator satisfies the application-side Effects
// port structurally, so the run Coordinator drives it without an adapter.
var _ runs.Effects = (*runsegment.Effects)(nil)

func (s *Server) runSegmentEffects() *runsegment.Effects {
	publish := func(cwd string, paths []string) {
		s.PublishWorkspaceEvent(protocol.WorkspaceEvent{
			Type:  protocol.WorkspaceEventFilesChanged,
			Cwd:   cwd,
			Paths: paths,
		})
	}
	if s.rt == nil {
		return runsegment.New(runsegment.Config{
			Checkpoints:        s.checkpoints,
			PublishFileChanges: publish,
		})
	}
	return s.rt.RunSegmentEffects(s.checkpoints, publish)
}
