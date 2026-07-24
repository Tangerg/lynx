package workspace

// AgentDocFile is the content read from one discovered AGENTS.md source. The
// prompt-source adapter owns discovery; the agent-execution adapter owns prompt
// rendering. This application value carries only the data exchanged between
// those two use cases.
type AgentDocFile struct {
	Path    string
	Content string
}

// RecipeScope identifies the source layer that supplied a recipe.
type RecipeScope string

const (
	RecipeScopeProject RecipeScope = "project"
	RecipeScopeGlobal  RecipeScope = "global"
)

// Recipe is a discovered prompt template. Filesystem layout and frontmatter
// parsing belong to the prompt-source adapter; placeholder expansion is a
// client concern.
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
	SkillScopeGlobal  SkillScope = "global"
)

// SkillInfo is one skill visible to a workspace, including the source layer
// selected by prompt-source precedence.
type SkillInfo struct {
	Name        string
	Description string
	Scope       SkillScope
}
