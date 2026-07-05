package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/recipes"
)

// ListRecipes enumerates the prompt recipes visible from cwd — project recipes
// (<cwd>/.lyra/recipes) layered over the global directory, project winning on a
// name collision (workspace.recipes.list). The client expands a chosen recipe's
// body and sends it as a turn; the runtime only discovers them.
func (r *Runtime) ListRecipes(ctx context.Context, cwd string) ([]recipes.Recipe, error) {
	return recipes.List(ctx, recipes.ProjectDir(cwd), r.recipesGlobalDir)
}
