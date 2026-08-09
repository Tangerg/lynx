package promptsource

import (
	"context"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
)

// WorkspaceRecipes lists project recipes layered over one configured global
// directory.
type WorkspaceRecipes struct{ userDir string }

// NewWorkspaceRecipes returns the workspace discovery adapter for recipes.
func NewWorkspaceRecipes(userDir string) WorkspaceRecipes {
	return WorkspaceRecipes{userDir: userDir}
}

var _ workspaceapp.RecipeLister = WorkspaceRecipes{}

func (l WorkspaceRecipes) List(ctx context.Context, cwd string) ([]workspaceapp.Recipe, error) {
	return listRecipes(ctx, recipeDir(cwd), l.userDir)
}

// WorkspaceSkills lists project Skills layered over one configured user
// directory.
type WorkspaceSkills struct{ userDir string }

// NewWorkspaceSkills returns the workspace Skill-discovery adapter.
func NewWorkspaceSkills(userDir string) WorkspaceSkills {
	return WorkspaceSkills{userDir: userDir}
}

var _ workspaceapp.SkillCatalog = WorkspaceSkills{}

func (c WorkspaceSkills) List(ctx context.Context, cwd string) ([]workspaceapp.SkillSummary, error) {
	return ListSkills(ctx, ProjectSkillDir(cwd), c.userDir)
}
