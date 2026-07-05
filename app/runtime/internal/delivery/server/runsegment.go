package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/runsegment"
)

func (s *Server) runSegmentEffects() *runsegment.Effects {
	var processes runsegment.ProcessLookup
	if s.rt != nil {
		processes = s.turns()
	}
	return runsegment.New(runsegment.Config{
		Stores:      s.rt,
		Processes:   processes,
		Checkpoints: s.workspace,
		PublishFileChanges: func(cwd string, paths []string) {
			s.PublishWorkspaceEvent(protocol.WorkspaceEvent{
				Type:  protocol.WorkspaceEventFilesChanged,
				Cwd:   cwd,
				Paths: paths,
			})
		},
	})
}
