package protocol

// SkillSource is where a discovered Skill came from (API.md §4.10): project
// (<cwd>/.lyra/skills) or global (<LYRA_HOME>/skills).
type SkillSource string

const (
	SkillSourceProject SkillSource = "project"
	SkillSourceGlobal  SkillSource = "global"
)

// Skill is one entry in skills.discovered.list (API.md §4.10).
type Skill struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Source      SkillSource `json:"source,omitempty"` // see SkillSource
}

// SkillLifecycle is a managed skill's curator state (skills.library.list):
// active (loadable by the agent) or archived (preserved, not loaded).
type SkillLifecycle string

const (
	SkillLifecycleActive   SkillLifecycle = "active"
	SkillLifecycleArchived SkillLifecycle = "archived"
)

// ManagedSkill is one entry in the global self-authored skill library
// (skills.library.list), tagged with its curator lifecycle. Distinct from
// [Skill] (the agent's project+global discovery view): this is the management
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

// AgentDocScope is where an AGENTS.md was discovered in the cwd→home hierarchy
// (API.md §4.10). Mirrors MemoryScope's values but is a distinct domain (left
// separate rather than DRY-coupled — two scopes is under the rule-of-three).
type AgentDocScope string

const (
	AgentDocScopeCwd         AgentDocScope = "cwd"
	AgentDocScopeProjectRoot AgentDocScope = "projectRoot"
	AgentDocScopeHome        AgentDocScope = "home"
)

// AgentDoc is one AGENTS.md discovered from cwd upward (API.md §4.10).
type AgentDoc struct {
	Path  string        `json:"path"`
	Title string        `json:"title,omitempty"`
	Scope AgentDocScope `json:"scope"` // see AgentDocScope
}
