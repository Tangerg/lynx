package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/runsegment"
)

func (s *Server) runSegmentEffects() *runsegment.Effects {
	publish := func(cwd string, paths []string) {
		s.PublishWorkspaceEvent(protocol.WorkspaceEvent{
			Type:  protocol.WorkspaceEventFilesChanged,
			Cwd:   cwd,
			Paths: paths,
		})
	}
	if s.runSegments == nil {
		return runsegment.New(runsegment.Config{
			Checkpoints:        s.checkpoints,
			PublishFileChanges: publish,
		})
	}
	return s.runSegments.RunSegmentEffects(s.checkpoints, publish)
}
