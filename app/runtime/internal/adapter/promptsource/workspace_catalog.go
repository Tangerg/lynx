package promptsource

import (
	"context"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/recipes"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

// WorkspaceRecipes lists project recipes layered over one configured global
// directory.
type WorkspaceRecipes struct{ globalDir string }

// NewWorkspaceRecipes returns the workspace discovery adapter for recipes.
func NewWorkspaceRecipes(globalDir string) WorkspaceRecipes {
	return WorkspaceRecipes{globalDir: globalDir}
}

var _ workspaceapp.RecipeLister = WorkspaceRecipes{}

func (l WorkspaceRecipes) List(ctx context.Context, cwd string) ([]recipes.Recipe, error) {
	return ListRecipes(ctx, recipes.ProjectDir(cwd), l.globalDir)
}

// WorkspaceSkills lists project skills layered over one configured global
// directory.
type WorkspaceSkills struct{ globalDir string }

// NewWorkspaceSkills returns the workspace skill-discovery adapter.
func NewWorkspaceSkills(globalDir string) WorkspaceSkills {
	return WorkspaceSkills{globalDir: globalDir}
}

var _ workspaceapp.SkillCatalog = WorkspaceSkills{}

func (c WorkspaceSkills) ListSkills(ctx context.Context, cwd string) ([]skills.Info, error) {
	return ListSkills(ctx, skills.ProjectDir(cwd), c.globalDir)
}
