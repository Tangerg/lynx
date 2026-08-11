package runtimeembedded

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/authoringcontext"
)

type authoringContextBinding interface {
	ListAgentDocs(context.Context, protocol.WorkspaceQuery, embedded.CallOptions) (*protocol.Page[protocol.AgentDoc], error)
	ListRecipes(context.Context, protocol.WorkspaceQuery, embedded.CallOptions) (*protocol.Page[protocol.Recipe], error)
}

type authoringContextAdapter struct{ runtime *Runtime }

var _ authoringcontext.Service = (*authoringContextAdapter)(nil)

func (adapter *authoringContextAdapter) Documents(ctx context.Context, workspace string) ([]authoringcontext.Document, error) {
	r := adapter.runtime
	query, err := authoringWorkspaceQuery(workspace)
	if err != nil {
		return nil, err
	}
	page, err := r.authoringContext.ListAgentDocs(ctx, query, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if page == nil {
		return nil, errors.New("list agent documents: runtime returned nil")
	}
	if page.NextCursor != "" {
		return nil, errors.New("list agent documents: runtime returned an unusable continuation cursor")
	}
	documents := make([]authoringcontext.Document, 0, len(page.Data))
	seen := make(map[string]struct{}, len(page.Data))
	for index, value := range page.Data {
		document := authoringcontext.Document{Path: value.Path, Title: value.Title, Scope: authoringcontext.DocumentScope(value.Scope)}
		if err := document.Validate(); err != nil {
			return nil, fmt.Errorf("list agent documents item %d: %w", index+1, err)
		}
		if _, duplicate := seen[document.Path]; duplicate {
			return nil, fmt.Errorf("list agent documents repeats %q", document.Path)
		}
		seen[document.Path] = struct{}{}
		documents = append(documents, document)
	}
	return documents, nil
}

func (adapter *authoringContextAdapter) Recipes(ctx context.Context, workspace string) ([]authoringcontext.Recipe, error) {
	r := adapter.runtime
	query, err := authoringWorkspaceQuery(workspace)
	if err != nil {
		return nil, err
	}
	page, err := r.authoringContext.ListRecipes(ctx, query, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if page == nil {
		return nil, errors.New("list recipes: runtime returned nil")
	}
	if page.NextCursor != "" {
		return nil, errors.New("list recipes: runtime returned an unusable continuation cursor")
	}
	recipes := make([]authoringcontext.Recipe, 0, len(page.Data))
	seen := make(map[string]struct{}, len(page.Data))
	for index, value := range page.Data {
		recipe := authoringcontext.Recipe{
			Name: value.Name, Description: value.Description, ArgumentHint: value.ArgumentHint,
			Body: value.Body, Scope: authoringcontext.RecipeScope(value.Scope), Source: value.Source,
		}
		if err := recipe.Validate(); err != nil {
			return nil, fmt.Errorf("list recipes item %d: %w", index+1, err)
		}
		if _, duplicate := seen[recipe.Name]; duplicate {
			return nil, fmt.Errorf("list recipes repeats %q", recipe.Name)
		}
		seen[recipe.Name] = struct{}{}
		recipes = append(recipes, recipe)
	}
	return recipes, nil
}

func authoringWorkspaceQuery(workspace string) (protocol.WorkspaceQuery, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return protocol.WorkspaceQuery{}, errors.New("authoring context: workspace is empty")
	}
	return protocol.WorkspaceQuery{Workspace: protocol.WorkspaceRef{Path: workspace}}, nil
}
