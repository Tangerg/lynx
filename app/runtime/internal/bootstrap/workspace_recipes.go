package bootstrap

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/promptsource"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/recipes"
)

// recipeLister is the composition-root binding of the workspace coordinator's
// RecipeLister port to the filesystem discovery adapter: it layers a session's
// project recipes (<cwd>/.lyra/recipes) over the configured global directory.
type recipeLister struct{ globalDir string }

func (l recipeLister) List(ctx context.Context, cwd string) ([]recipes.Recipe, error) {
	return promptsource.ListRecipes(ctx, recipes.ProjectDir(cwd), l.globalDir)
}
