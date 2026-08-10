package protocol

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
