package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListKnowledge returns the effective knowledge cascade for a workspace.
func (r *Runtime) ListKnowledge(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.KnowledgeEntry], error) {
	return invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.KnowledgeEntry]](ctx, r, "knowledge.list", request, callOptions(options))
}

// GetKnowledge returns one knowledge entry.
func (r *Runtime) GetKnowledge(ctx context.Context, request protocol.GetKnowledgeRequest, options CallOptions) (*protocol.KnowledgeEntry, error) {
	return invoke[protocol.GetKnowledgeRequest, *protocol.KnowledgeEntry](ctx, r, "knowledge.get", request, callOptions(options))
}

// UpdateKnowledge replaces one user-editable knowledge entry.
func (r *Runtime) UpdateKnowledge(ctx context.Context, request protocol.UpdateKnowledgeRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "knowledge.update", request, commandOptions(options))
}

// ListDiscoveredSkills returns Skills applicable to a workspace.
func (r *Runtime) ListDiscoveredSkills(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.Skill], error) {
	return invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.Skill]](ctx, r, "skills.discovered.list", request, callOptions(options))
}

// ListManagedSkills returns user-scope Skills managed by the Runtime.
func (r *Runtime) ListManagedSkills(ctx context.Context, options CallOptions) (*protocol.Page[protocol.ManagedSkill], error) {
	return invoke[struct{}, *protocol.Page[protocol.ManagedSkill]](ctx, r, "skills.library.list", struct{}{}, callOptions(options))
}

// ArchiveSkill removes a managed Skill from active discovery.
func (r *Runtime) ArchiveSkill(ctx context.Context, request protocol.SkillNameRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "skills.library.archive", request, commandOptions(options))
}

// RestoreSkill restores an archived managed Skill.
func (r *Runtime) RestoreSkill(ctx context.Context, request protocol.SkillNameRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "skills.library.restore", request, commandOptions(options))
}

// ListSkillProposals returns pending Skill proposals for a workspace.
func (r *Runtime) ListSkillProposals(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.SkillProposal], error) {
	return invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.SkillProposal]](ctx, r, "skills.proposals.list", request, callOptions(options))
}

// ApproveSkillProposal accepts one proposed Skill.
func (r *Runtime) ApproveSkillProposal(ctx context.Context, request protocol.SkillProposalRef, options CommandOptions) error {
	return invokeAck(ctx, r, "skills.proposals.approve", request, commandOptions(options))
}

// RejectSkillProposal rejects one proposed Skill.
func (r *Runtime) RejectSkillProposal(ctx context.Context, request protocol.SkillProposalRef, options CommandOptions) error {
	return invokeAck(ctx, r, "skills.proposals.reject", request, commandOptions(options))
}

// ListRecipes returns recipes applicable to a workspace.
func (r *Runtime) ListRecipes(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.Recipe], error) {
	return invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.Recipe]](ctx, r, "recipes.list", request, callOptions(options))
}

// ListAgentDocs returns agent instruction documents applicable to a workspace.
func (r *Runtime) ListAgentDocs(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.AgentDoc], error) {
	return invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.AgentDoc]](ctx, r, "agentDocs.list", request, callOptions(options))
}
