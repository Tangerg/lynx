package runtimeembedded

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/scope/app/runtime/embedded"
	"github.com/Tangerg/scope/app/runtime/protocol"

	"github.com/Tangerg/scope/app/cli/internal/authoringcontext"
)

type authoringContextBinding interface {
	ListAgentDocs(context.Context, protocol.WorkspaceQuery, embedded.CallOptions) (*protocol.Page[protocol.AgentDoc], error)
	ListRecipes(context.Context, protocol.WorkspaceQuery, embedded.CallOptions) (*protocol.Page[protocol.Recipe], error)
}

type authoringContextAdapter struct{ runtime *Runtime }

var _ authoringcontext.Service = (*authoringContextAdapter)(nil)

func (a *authoringContextAdapter) Documents(ctx context.Context, workspace string) ([]authoringcontext.Document, error) {
	r := a.runtime
	query, err := authoringWorkspaceQuery(workspace)
	if err != nil {
		return nil, err
	}
	page, err := r.authoringContext.ListAgentDocs(ctx, query, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list agent documents", page)
	if err != nil {
		return nil, err
	}
	documents := make([]authoringcontext.Document, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		document := authoringcontext.Document{Path: value.Path, Title: value.Title, Scope: authoringcontext.DocumentScope(value.Scope)}
		if err := document.Validate(); err != nil {
			return nil, runtimeContractViolation("list agent documents item %d is invalid: %v", index+1, err)
		}
		if _, duplicate := seen[document.Path]; duplicate {
			return nil, runtimeContractViolation("list agent documents repeats %q", document.Path)
		}
		seen[document.Path] = struct{}{}
		documents = append(documents, document)
	}
	return documents, nil
}

func (a *authoringContextAdapter) Recipes(ctx context.Context, workspace string) ([]authoringcontext.Recipe, error) {
	r := a.runtime
	query, err := authoringWorkspaceQuery(workspace)
	if err != nil {
		return nil, err
	}
	page, err := r.authoringContext.ListRecipes(ctx, query, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list recipes", page)
	if err != nil {
		return nil, err
	}
	recipes := make([]authoringcontext.Recipe, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		recipe := authoringcontext.Recipe{
			Name: value.Name, Description: value.Description, ArgumentHint: value.ArgumentHint,
			Body: value.Body, Scope: authoringcontext.RecipeScope(value.Scope), Source: value.Source,
		}
		if err := recipe.Validate(); err != nil {
			return nil, runtimeContractViolation("list recipes item %d is invalid: %v", index+1, err)
		}
		if _, duplicate := seen[recipe.Name]; duplicate {
			return nil, runtimeContractViolation("list recipes repeats %q", recipe.Name)
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
