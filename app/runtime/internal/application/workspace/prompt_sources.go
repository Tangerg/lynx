package workspace

// AgentDocFile is the content read from one discovered AGENTS.md source. It
// carries only the source identity and content; discovery and prompt rendering
// remain independent concerns.
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
