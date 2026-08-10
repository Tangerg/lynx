package server

import (
	"context"
	"fmt"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListRecipes maps application recipe discovery to the protocol shape.
func (s *Server) ListRecipes(ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.Recipe], error) {
	found, err := s.workspaceDiscovery.Recipes(ctx, in.Workspace.Path)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.Recipe, 0, len(found))
	for _, recipe := range found {
		scope, ok := presentRecipeScope(recipe.Scope)
		if !ok {
			return nil, fmt.Errorf("recipes.list: unsupported recipe scope %q", recipe.Scope)
		}
		out = append(out, protocol.Recipe{
			Name: recipe.Name, Description: recipe.Description, ArgumentHint: recipe.ArgumentHint,
			Body: recipe.Body, Scope: scope, Source: recipe.Source,
		})
	}
	return protocol.NewPage(out), nil
}

func presentRecipeScope(scope workspaceapp.RecipeScope) (protocol.RecipeScope, bool) {
	switch scope {
	case workspaceapp.RecipeScopeProject:
		return protocol.RecipeScopeProject, true
	case workspaceapp.RecipeScopeGlobal:
		return protocol.RecipeScopeGlobal, true
	default:
		return "", false
	}
}
