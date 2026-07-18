package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// WorkspaceListManagedSkills returns the global self-authored skill library —
// active and archived skills, each tagged with its lifecycle
// (workspace.skills.list). The library is small, so it comes back in one page
// (same as workspace.listSkills). Empty when no authoring store is wired.
func (s *Server) WorkspaceListManagedSkills(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.ManagedSkill], error) {
	entries, err := s.workspace.ListManagedSkills(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.ManagedSkill, 0, len(entries))
	for _, e := range entries {
		out = append(out, protocol.ManagedSkill{
			Name:        e.Name,
			Description: e.Description,
			Lifecycle:   protocol.SkillLifecycle(e.Lifecycle),
		})
	}
	return protocol.NewPage(out), nil
}

// WorkspaceArchiveSkill removes a skill from active use without deleting it
// (workspace.skills.archive), then fans out skills.changed so open clients
// refresh.
func (s *Server) WorkspaceArchiveSkill(ctx context.Context, in protocol.SkillNameRequest) error {
	if in.Name == "" {
		return protocol.ErrInvalidParams
	}
	if err := s.workspace.ArchiveSkill(ctx, in.Name); err != nil {
		return err
	}
	s.PublishWorkspaceEvent(protocol.WorkspaceEvent{Type: protocol.WorkspaceEventSkillsChanged})
	return nil
}

// WorkspaceRestoreSkill returns an archived skill to active use
// (workspace.skills.restore), then fans out skills.changed.
func (s *Server) WorkspaceRestoreSkill(ctx context.Context, in protocol.SkillNameRequest) error {
	if in.Name == "" {
		return protocol.ErrInvalidParams
	}
	if err := s.workspace.RestoreSkill(ctx, in.Name); err != nil {
		return err
	}
	s.PublishWorkspaceEvent(protocol.WorkspaceEvent{Type: protocol.WorkspaceEventSkillsChanged})
	return nil
}
