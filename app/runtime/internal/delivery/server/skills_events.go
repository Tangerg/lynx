package server

import "github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"

// observeSkillChanges installs the delivery side of the composition-root skill
// refresh bridge. The application has already completed the durable mutation;
// Delivery only translates the payload-free nudge to its protocol event.
func (s *Server) observeSkillChanges(src SkillChangeSource) {
	src.Observe(func(struct{}) {
		s.PublishWorkspaceEvent(protocol.WorkspaceEvent{Type: protocol.WorkspaceEventSkillsChanged})
	})
}
