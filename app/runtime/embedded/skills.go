package embedded

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// ListDiscoveredSkills returns Skills applicable to a workspace.
func (r *Runtime) ListDiscoveredSkills(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.Skill], error) {
	return r.invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.Skill]](ctx, operation.SkillsDiscoveredList, request, callOptions(options))
}

// ListManagedSkills returns user-scope Skills managed by the Runtime.
func (r *Runtime) ListManagedSkills(ctx context.Context, options CallOptions) (*protocol.Page[protocol.ManagedSkill], error) {
	return r.invoke[struct{}, *protocol.Page[protocol.ManagedSkill]](ctx, operation.SkillsLibraryList, struct{}{}, callOptions(options))
}

// ArchiveSkill removes a managed Skill from active discovery.
func (r *Runtime) ArchiveSkill(ctx context.Context, request protocol.SkillNameRequest, options CommandOptions) error {
	return r.invokeAck(ctx, operation.SkillsLibraryArchive, request, commandOptions(options))
}

// RestoreSkill restores an archived managed Skill.
func (r *Runtime) RestoreSkill(ctx context.Context, request protocol.SkillNameRequest, options CommandOptions) error {
	return r.invokeAck(ctx, operation.SkillsLibraryRestore, request, commandOptions(options))
}

// ListSkillProposals returns pending Skill proposals for a workspace.
func (r *Runtime) ListSkillProposals(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.SkillProposal], error) {
	return r.invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.SkillProposal]](ctx, operation.SkillsProposalsList, request, callOptions(options))
}

// ApproveSkillProposal accepts one proposed Skill.
func (r *Runtime) ApproveSkillProposal(ctx context.Context, request protocol.SkillProposalRef, options CommandOptions) error {
	return r.invokeAck(ctx, operation.SkillsProposalsApprove, request, commandOptions(options))
}

// RejectSkillProposal rejects one proposed Skill.
func (r *Runtime) RejectSkillProposal(ctx context.Context, request protocol.SkillProposalRef, options CommandOptions) error {
	return r.invokeAck(ctx, operation.SkillsProposalsReject, request, commandOptions(options))
}
