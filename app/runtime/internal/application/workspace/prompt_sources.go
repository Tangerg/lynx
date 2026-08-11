package workspace

// AgentDocScope identifies the layer that supplied an instruction document.
// It is determined while walking the cascade and retained with the discovered
// value so later use cases never reconstruct semantic scope from a path.
type AgentDocScope string

const (
	AgentDocScopeHome        AgentDocScope = "home"
	AgentDocScopeCWD         AgentDocScope = "cwd"
	AgentDocScopeProjectRoot AgentDocScope = "projectRoot"
)

// AgentDocFile is the content read from one discovered AGENTS.md source. It
// carries source identity, cascade provenance, and content; prompt rendering
// remains an independent concern.
type AgentDocFile struct {
	Path    string
	Content string
	Scope   AgentDocScope
}

// RecipeScope identifies the source layer that supplied a recipe.
type RecipeScope string

const (
	RecipeScopeProject RecipeScope = "project"
	RecipeScopeGlobal  RecipeScope = "global"
)

// Recipe is a discovered prompt template. Source layout and frontmatter have
// already been resolved; placeholder expansion belongs to the consumer.
type Recipe struct {
	Name         string
	Description  string
	ArgumentHint string
	Body         string
	Scope        RecipeScope
	Source       string
}

// SkillScope identifies the source layer selected by prompt-source precedence.
type SkillScope string

const (
	SkillScopeProject SkillScope = "project"
	SkillScopeUser    SkillScope = "user"
)

// SkillSummary is one skill visible to a workspace, including the source layer
// selected by prompt-source precedence.
type SkillSummary struct {
	Name        string
	Description string
	Scope       SkillScope
}
