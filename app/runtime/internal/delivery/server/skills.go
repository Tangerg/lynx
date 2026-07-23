package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

// ListManagedSkills returns the global self-authored skill library —
// active and archived skills, each tagged with its lifecycle
// (skills.library.list). The library is small, so it comes back in one page
// (same as skills.discovered.list). Empty when no authoring store is wired.
func (s *Server) ListManagedSkills(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.ManagedSkill], error) {
	entries, err := s.workspaceSkills.ListManagedSkills(ctx)
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

// ArchiveSkill removes a skill from active use without deleting it
// (skills.library.archive). The application use case publishes the refresh
// nudge after its durable mutation commits.
func (s *Server) ArchiveSkill(ctx context.Context, in protocol.SkillNameRequest) error {
	if in.Name == "" {
		return protocol.ErrInvalidParams
	}
	if err := s.workspaceSkills.ArchiveSkill(ctx, in.Name); err != nil {
		return err
	}
	return nil
}

// RestoreSkill returns an archived skill to active use
// (skills.library.restore). The application use case publishes the refresh
// nudge after its durable mutation commits.
func (s *Server) RestoreSkill(ctx context.Context, in protocol.SkillNameRequest) error {
	if in.Name == "" {
		return protocol.ErrInvalidParams
	}
	if err := s.workspaceSkills.RestoreSkill(ctx, in.Name); err != nil {
		return err
	}
	return nil
}

// ListSkillDrafts returns the agent-mined skill proposals awaiting
// review (skills.drafts.list). The draft area is small, so it comes back in one
// page. capability_not_negotiated when authoring is disabled.
func (s *Server) ListSkillDrafts(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.SkillDraft], error) {
	drafts, err := s.workspaceSkills.ListSkillDrafts(ctx)
	if err != nil {
		return nil, mapSkillDraftErr(err, "skills.drafts.list")
	}
	out := make([]protocol.SkillDraft, 0, len(drafts))
	for _, d := range drafts {
		out = append(out, protocol.SkillDraft{
			Name:          d.Handle.Name,
			Revision:      d.Handle.Revision,
			Description:   d.Description,
			CreatedBy:     d.CreatedBy,
			SourceSession: d.SourceSession,
		})
	}
	return protocol.NewPage(out), nil
}

// PromoteSkillDraft publishes a reviewed draft into the active library
// (skills.drafts.promote). The application use case publishes the refresh nudge
// after the active library changes.
func (s *Server) PromoteSkillDraft(ctx context.Context, in protocol.SkillDraftRef) error {
	handle, err := skillDraftHandle(in)
	if err != nil {
		return err
	}
	if err := s.workspaceSkills.PromoteSkillDraft(ctx, handle); err != nil {
		return mapSkillDraftErr(err, "skills.drafts.promote")
	}
	return nil
}

// RejectSkillDraft discards a reviewed draft (skills.drafts.reject).
func (s *Server) RejectSkillDraft(ctx context.Context, in protocol.SkillDraftRef) error {
	handle, err := skillDraftHandle(in)
	if err != nil {
		return err
	}
	return mapSkillDraftErr(s.workspaceSkills.RejectSkillDraft(ctx, handle), "skills.drafts.reject")
}

// skillDraftHandle validates the wire ref and reconstructs the content-addressed
// draft handle. Name and Revision are both required.
func skillDraftHandle(in protocol.SkillDraftRef) (skills.DraftHandle, error) {
	if in.Name == "" || in.Revision == "" {
		return skills.DraftHandle{}, fmt.Errorf("%w: name and revision are required", protocol.ErrInvalidParams)
	}
	return skills.DraftHandle{Name: in.Name, Revision: in.Revision}, nil
}

func mapSkillDraftErr(err error, method string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, workspace.ErrSkillDraftsUnavailable):
		return capabilityNotNegotiated(method)
	case errors.Is(err, skills.ErrConflict):
		return fmt.Errorf("%w: a skill with that name already exists", protocol.ErrInvalidParams)
	case errors.Is(err, skills.ErrDraftChanged):
		return fmt.Errorf("%w: the staged draft changed", protocol.ErrInvalidParams)
	case errors.Is(err, skills.ErrNotFound):
		return fmt.Errorf("%w: no such draft", protocol.ErrInvalidParams)
	default:
		return err
	}
}
