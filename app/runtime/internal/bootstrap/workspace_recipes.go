package bootstrap

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/promptsource"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/recipes"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

// recipeLister is the composition-root binding of the workspace coordinator's
// RecipeLister port to the filesystem discovery adapter: it layers a session's
// project recipes (<cwd>/.lyra/recipes) over the configured global directory.
type recipeLister struct{ globalDir string }

func (l recipeLister) List(ctx context.Context, cwd string) ([]recipes.Recipe, error) {
	return promptsource.ListRecipes(ctx, recipes.ProjectDir(cwd), l.globalDir)
}

type skillCatalog struct{ globalDir string }

func (c skillCatalog) ListSkills(ctx context.Context, cwd string) ([]skills.Info, error) {
	return promptsource.ListSkills(ctx, skills.ProjectDir(cwd), c.globalDir)
}
