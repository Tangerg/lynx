package protocol

import "context"

// Recipes is the recipes.* method group.
type Recipes interface {
	ListRecipes(ctx context.Context, in WorkspaceListQuery) (*Page[Recipe], error)
}

// Recipe-discovery wire types (recipes.list, API.md §7.5). A recipe is
// a user-invoked, parameterized prompt template discovered from .lyra/recipes
// (project) layered over the global recipes directory. The client renders the
// list, expands a chosen recipe's body ($ARGUMENTS / $1..$9) with the user's
// input, and sends the result as a turn — the runtime only discovers them.

// RecipeScope is where a discovered Recipe came from: project (<cwd>/.lyra/
// recipes) or global (<LYRA_HOME>/recipes). Mirrors SkillSource's values but is
// a distinct domain (left separate rather than DRY-coupled — under rule-of-three).
type RecipeScope string

const (
	RecipeScopeProject RecipeScope = "project"
	RecipeScopeGlobal  RecipeScope = "global"
)

// Recipe is one entry in recipes.list (API.md §4.10). Body is the full
// prompt template (the client expands and sends it), so it travels with the
// listing — recipes are small. ArgumentHint is the optional placeholder shown in
// the slash autocomplete (e.g. "[focus area]").
type Recipe struct {
	Name         string      `json:"name"`
	Description  string      `json:"description,omitempty"`
	ArgumentHint string      `json:"argumentHint,omitempty"`
	Body         string      `json:"body"`
	Scope        RecipeScope `json:"scope"`  // see RecipeScope
	Source       string      `json:"source"` // absolute path of the .md file
}
