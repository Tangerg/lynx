package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

// ListDiscoveredSkills maps application skill discovery to the protocol shape.
func (s *Server) ListDiscoveredSkills(ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.Skill], error) {
	found, err := s.workspaceSkills.List(ctx, in.Workspace.Path)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.Skill, 0, len(found))
	for _, skill := range found {
		scope, ok := presentWorkspaceSkillScope(skill.Scope)
		if !ok {
			return nil, fmt.Errorf("skills.discovered.list: unsupported skill scope %q", skill.Scope)
		}
		out = append(out, protocol.Skill{Name: skill.Name, Description: skill.Description, Scope: scope})
	}
	return protocol.NewPage(out), nil
}

func presentWorkspaceSkillScope(scope workspace.SkillScope) (protocol.SkillScope, bool) {
	switch scope {
	case workspace.SkillScopeProject:
		return protocol.SkillScopeProject, true
	case workspace.SkillScopeUser:
		return protocol.SkillScopeUser, true
	default:
		return "", false
	}
}

// ListManagedSkills returns the user self-authored Skill library —
// active and archived skills, each tagged with its lifecycle
// (skills.library.list). The library is small, so it comes back in one page
// (same as skills.discovered.list). capability_not_negotiated when the library
// curator is disabled.
func (s *Server) ListManagedSkills(ctx context.Context) (*protocol.Page[protocol.ManagedSkill], error) {
	entries, err := s.workspaceSkills.Managed(ctx)
	if err != nil {
		return nil, mapSkillLibraryErr(err, "skills.library.list")
	}
	out := make([]protocol.ManagedSkill, 0, len(entries))
	for _, e := range entries {
		lifecycle, ok := presentSkillLifecycle(e.Lifecycle)
		if !ok {
			return nil, fmt.Errorf("skills.library.list: unsupported lifecycle %q", e.Lifecycle)
		}
		out = append(out, protocol.ManagedSkill{
			Name:        e.Name,
			Description: e.Description,
			Lifecycle:   lifecycle,
		})
	}
	return protocol.NewPage(out), nil
}

func presentSkillLifecycle(lifecycle skills.Lifecycle) (protocol.SkillLifecycle, bool) {
	switch lifecycle {
	case skills.Active:
		return protocol.SkillLifecycleActive, true
	case skills.Archived:
		return protocol.SkillLifecycleArchived, true
	default:
		return "", false
	}
}

// ArchiveSkill removes a skill from active use without deleting it
// (skills.library.archive). The application use case publishes the refresh
// nudge after its durable mutation commits.
func (s *Server) ArchiveSkill(ctx context.Context, in protocol.SkillNameRequest) error {
	if in.Name == "" {
		return protocol.ErrInvalidParams
	}
	if err := s.workspaceSkills.Archive(ctx, in.Name); err != nil {
		return mapSkillLibraryErr(err, "skills.library.archive")
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
	if err := s.workspaceSkills.Restore(ctx, in.Name); err != nil {
		return mapSkillLibraryErr(err, "skills.library.restore")
	}
	return nil
}

// ListSkillProposals returns complete project and user proposals awaiting
// review (skills.proposals.list).
func (s *Server) ListSkillProposals(ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.SkillProposal], error) {
	proposals, err := s.workspaceSkills.Proposals(ctx, in.Workspace.Path)
	if err != nil {
		return nil, mapSkillProposalErr(err, "skills.proposals.list")
	}
	out := make([]protocol.SkillProposal, 0, len(proposals))
	for _, proposal := range proposals {
		scope, ok := presentSkillProposalScope(proposal.Ref.Scope)
		if !ok {
			return nil, fmt.Errorf("skills.proposals.list: unsupported scope %q", proposal.Ref.Scope)
		}
		origin, ok := presentSkillProposalOrigin(proposal.Origin)
		if !ok {
			return nil, fmt.Errorf("skills.proposals.list: unsupported origin %q", proposal.Origin)
		}
		out = append(out, protocol.SkillProposal{
			Name:          proposal.Ref.Name,
			Revision:      proposal.Ref.Revision,
			Scope:         scope,
			Description:   proposal.Description,
			Instructions:  proposal.Instructions,
			Origin:        origin,
			SourceSession: proposal.SourceSession,
			Revises:       proposal.Revises,
		})
	}
	return protocol.NewPage(out), nil
}

// ApproveSkillProposal activates exactly the reviewed immutable proposal.
func (s *Server) ApproveSkillProposal(ctx context.Context, in protocol.SkillProposalRef) error {
	ref, err := skillProposalRef(in)
	if err != nil {
		return err
	}
	return mapSkillProposalErr(
		s.workspaceSkills.ApproveProposal(ctx, in.Workspace.Path, ref),
		"skills.proposals.approve",
	)
}

// RejectSkillProposal removes exactly the reviewed immutable proposal.
func (s *Server) RejectSkillProposal(ctx context.Context, in protocol.SkillProposalRef) error {
	ref, err := skillProposalRef(in)
	if err != nil {
		return err
	}
	return mapSkillProposalErr(
		s.workspaceSkills.RejectProposal(ctx, in.Workspace.Path, ref),
		"skills.proposals.reject",
	)
}

func skillProposalRef(in protocol.SkillProposalRef) (skills.ProposalRef, error) {
	scope, ok := proposalScopeDomain(in.Scope)
	if !ok {
		return skills.ProposalRef{}, fmt.Errorf("%w: scope must be project or user", protocol.ErrInvalidParams)
	}
	ref := skills.ProposalRef{Scope: scope, Name: in.Name, Revision: in.Revision}
	if err := ref.Validate(); err != nil {
		return skills.ProposalRef{}, fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	}
	return ref, nil
}

func presentSkillProposalScope(scope skills.Scope) (protocol.SkillScope, bool) {
	switch scope {
	case skills.ScopeProject:
		return protocol.SkillScopeProject, true
	case skills.ScopeUser:
		return protocol.SkillScopeUser, true
	default:
		return "", false
	}
}

func proposalScopeDomain(scope protocol.SkillScope) (skills.Scope, bool) {
	switch scope {
	case protocol.SkillScopeProject:
		return skills.ScopeProject, true
	case protocol.SkillScopeUser:
		return skills.ScopeUser, true
	default:
		return "", false
	}
}

func presentSkillProposalOrigin(origin skills.ProposalOrigin) (protocol.SkillProposalOrigin, bool) {
	switch origin {
	case "":
		return "", true
	case skills.ProposalOriginRequested:
		return protocol.SkillProposalOriginRequested, true
	case skills.ProposalOriginMined:
		return protocol.SkillProposalOriginMined, true
	default:
		return "", false
	}
}

func mapSkillProposalErr(err error, method string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, workspace.ErrSkillProposalsUnavailable):
		return capabilityNotNegotiated(method)
	case errors.Is(err, skills.ErrConflict):
		return fmt.Errorf("%w: a Skill with that name already exists", protocol.ErrInvalidParams)
	case errors.Is(err, skills.ErrProposalChanged):
		return fmt.Errorf("%w: the stored proposal changed", protocol.ErrInvalidParams)
	case errors.Is(err, skills.ErrNotFound):
		return fmt.Errorf("%w: no such proposal", protocol.ErrInvalidParams)
	default:
		return wireWorkspaceError(err)
	}
}

func mapSkillLibraryErr(err error, method string) error {
	if errors.Is(err, workspace.ErrSkillLibraryUnavailable) {
		return capabilityNotNegotiated(method)
	}
	return err
}
