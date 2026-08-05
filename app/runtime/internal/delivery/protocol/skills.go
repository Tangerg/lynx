package protocol

import "context"

// Skills is the skills.* method group. Discovery, the owned library, and
// proposals share the skill lifecycle rather than the workspace namespace.
type Skills interface {
	ListDiscoveredSkills(ctx context.Context, in WorkspaceQuery) (*Page[Skill], error)
	ListManagedSkills(ctx context.Context) (*Page[ManagedSkill], error)
	ArchiveSkill(ctx context.Context, in SkillNameRequest) error
	RestoreSkill(ctx context.Context, in SkillNameRequest) error
	ListSkillProposals(ctx context.Context, in WorkspaceQuery) (*Page[SkillProposal], error)
	ApproveSkillProposal(ctx context.Context, in SkillProposalRef) error
	RejectSkillProposal(ctx context.Context, in SkillProposalRef) error
}

// SkillScope identifies the project or user library that owns a Skill.
type SkillScope string

const (
	SkillScopeProject SkillScope = "project"
	SkillScopeUser    SkillScope = "user"
)

// Skill is one entry in skills.discovered.list (API.md §4.10).
type Skill struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Scope       SkillScope `json:"scope"`
}

// SkillLifecycle is a managed skill's curator state (skills.library.list):
// active (loadable by the agent) or archived (preserved, not loaded).
type SkillLifecycle string

const (
	SkillLifecycleActive   SkillLifecycle = "active"
	SkillLifecycleArchived SkillLifecycle = "archived"
)

// ManagedSkill is one entry in the user-scoped self-authored Skill library
// (skills.library.list), tagged with its curator lifecycle. Distinct from
// [Skill] (the Agent's project+user discovery view): this is the management
// surface, which also lists archived skills.
type ManagedSkill struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Lifecycle   SkillLifecycle `json:"lifecycle"`
}

// SkillNameRequest names the skill a skills.library.archive / restore call
// acts on.
type SkillNameRequest struct {
	Name string `json:"name"`
}
