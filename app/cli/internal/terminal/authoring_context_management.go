package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/authoringcontext"
)

func (a *app) ShowAgentDocuments() {
	if a.authoringContext == nil {
		a.message("this runtime composition has no authoring context service")
		return
	}
	workspace := a.session.Workspace
	a.runRuntimeReaderQuery("loading agent documents", authoringContextOperation, runtimeReaderAgentDocuments,
		func(ctx context.Context) (readerDocument, error) {
			documents, err := a.authoringContext.Documents(ctx, workspace)
			if err != nil {
				return readerDocument{}, err
			}
			return agentDocumentsDocument(workspace, documents), nil
		})
}

func agentDocumentsDocument(workspace string, documents []authoringcontext.Document) readerDocument {
	if len(documents) == 0 {
		return paragraphDocument("Agent documents", workspace, []string{"No AGENTS.md documents apply to this workspace."})
	}
	lines := make([]string, 0, len(documents))
	for _, document := range documents {
		label := string(document.Scope) + "  " + document.Path
		if document.Title != "" {
			label += "  · " + document.Title
		}
		lines = append(lines, label)
	}
	return paragraphDocument("Agent documents", fmt.Sprintf("%d applicable · %s", len(documents), workspace), lines)
}

func (a *app) ShowRecipes() {
	if a.authoringContext == nil {
		a.message("this runtime composition has no authoring context service")
		return
	}
	workspace := a.session.Workspace
	a.runRuntimeReaderQuery("loading recipes", authoringContextOperation, runtimeReaderRecipes,
		func(ctx context.Context) (readerDocument, error) {
			recipes, err := a.authoringContext.Recipes(ctx, workspace)
			if err != nil {
				return readerDocument{}, err
			}
			return recipesDocument(workspace, recipes), nil
		})
}

func recipesDocument(workspace string, recipes []authoringcontext.Recipe) readerDocument {
	if len(recipes) == 0 {
		return paragraphDocument("Prompt recipes", workspace, []string{"No prompt recipes are available."})
	}
	sections := make([]ToolSection, 0, len(recipes)*2)
	for _, recipe := range recipes {
		description := fallback(recipe.Description, "No description provided.")
		invocation := "/recipe " + recipe.Name
		if recipe.ArgumentHint != "" {
			invocation += " " + recipe.ArgumentHint
		}
		sections = append(sections,
			ToolSection{Title: recipe.Name + " · " + string(recipe.Scope), Style: toolSectionParagraph, Text: description + "\n" + invocation},
			ToolSection{Title: recipe.Source, Style: toolSectionCode, Language: "markdown", Text: recipe.Body},
		)
	}
	return readerDocument{Title: "Prompt recipes", Detail: fmt.Sprintf("%d available · %s", len(recipes), workspace), Sections: sections}
}

func (a *app) PrepareRecipe(argument string) error {
	if a.authoringContext == nil {
		return errors.New("this runtime composition has no authoring context service")
	}
	requested := strings.TrimSpace(argument)
	if requested == "" {
		return errors.New("usage: /recipe <name> [arguments]")
	}
	workspace := a.session.Workspace
	a.status.note("loading recipe")
	if !runOperation(a, authoringContextOperation, false,
		func(ctx context.Context) (expandedRecipe, error) {
			recipes, err := a.authoringContext.Recipes(ctx, workspace)
			if err != nil {
				return expandedRecipe{}, err
			}
			recipe, arguments, err := resolveRecipeInvocation(recipes, requested)
			if err != nil {
				return expandedRecipe{}, err
			}
			expanded, err := recipe.Expand(arguments)
			return expandedRecipe{recipe: recipe, prompt: expanded}, err
		},
		func(expanded expandedRecipe, err error) {
			if err != nil {
				a.message("prepare recipe failed: " + err.Error())
				return
			}
			a.openContextEditor(
				"Recipe · "+expanded.recipe.Name,
				"Review the expanded prompt. Enter inserts a newline; Ctrl+S sends or queues it.",
				expanded.prompt, "Expanded prompt",
				func(content string, complete func(error) bool) error {
					if strings.TrimSpace(content) == "" {
						return errors.New("recipe prompt is empty")
					}
					complete(nil)
					a.dispatchPrompt(agent.Message{Text: content})
					return nil
				},
			)
		},
	) {
		return errors.New("another authoring context operation is running")
	}
	return nil
}

type expandedRecipe struct {
	recipe authoringcontext.Recipe
	prompt string
}

func resolveRecipeInvocation(recipes []authoringcontext.Recipe, argument string) (authoringcontext.Recipe, string, error) {
	// Prefer the longest complete catalog name at the beginning of the argument.
	// This keeps every runtime-valid filename addressable, including names with
	// spaces, and gives "review code" precedence over recipe "review" + arg
	// "code" when both exist.
	matched := -1
	for index, recipe := range recipes {
		if _, ok := trimCommandIdentity(argument, recipe.Name); !ok {
			continue
		}
		if matched < 0 || len(recipe.Name) > len(recipes[matched].Name) {
			matched = index
		}
	}
	if matched >= 0 {
		recipe := recipes[matched]
		arguments, _ := trimCommandIdentity(argument, recipe.Name)
		return recipe, arguments, nil
	}
	identity, arguments, _ := splitCommandArgument(argument)
	recipe, err := resolveRecipe(recipes, identity)
	return recipe, arguments, err
}

func resolveRecipe(recipes []authoringcontext.Recipe, identity string) (authoringcontext.Recipe, error) {
	for _, recipe := range recipes {
		if recipe.Name == identity {
			return recipe, nil
		}
	}
	var matches []authoringcontext.Recipe
	for _, recipe := range recipes {
		if strings.HasPrefix(recipe.Name, identity) {
			matches = append(matches, recipe)
		}
	}
	switch len(matches) {
	case 0:
		return authoringcontext.Recipe{}, errors.New("recipe not found: " + identity)
	case 1:
		return matches[0], nil
	default:
		return authoringcontext.Recipe{}, errors.New("recipe name is ambiguous; use the full name")
	}
}
