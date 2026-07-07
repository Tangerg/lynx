package protocol

// SkillSource is where a discovered Skill came from (API.md §4.10): project
// (<cwd>/.lyra/skills) or global (<LYRA_HOME>/skills).
type SkillSource string

const (
	SkillSourceProject SkillSource = "project"
	SkillSourceGlobal  SkillSource = "global"
)

// Skill is one entry in workspace.listSkills (API.md §4.10).
type Skill struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Source      SkillSource `json:"source,omitempty"` // see SkillSource
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
