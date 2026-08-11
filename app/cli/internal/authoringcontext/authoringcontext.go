// Package authoringcontext defines discoverable instruction documents and
// parameterized prompt recipes available to a CLI session.
package authoringcontext

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type DocumentScope string

const (
	DocumentWorkingDirectory DocumentScope = "cwd"
	DocumentProjectRoot      DocumentScope = "projectRoot"
	DocumentHome             DocumentScope = "home"
)

func (scope DocumentScope) Validate() error {
	switch scope {
	case DocumentWorkingDirectory, DocumentProjectRoot, DocumentHome:
		return nil
	default:
		return fmt.Errorf("agent document scope %q is invalid", scope)
	}
}

type Document struct {
	Path  string
	Title string
	Scope DocumentScope
}

func (document Document) Validate() error {
	if strings.TrimSpace(document.Path) == "" {
		return errors.New("agent document path is empty")
	}
	return document.Scope.Validate()
}

type RecipeScope string

const (
	ProjectRecipe RecipeScope = "project"
	GlobalRecipe  RecipeScope = "global"
)

func (scope RecipeScope) Validate() error {
	if scope != ProjectRecipe && scope != GlobalRecipe {
		return fmt.Errorf("recipe scope %q is invalid", scope)
	}
	return nil
}

type Recipe struct {
	Name         string
	Description  string
	ArgumentHint string
	Body         string
	Scope        RecipeScope
	Source       string
}

func (recipe Recipe) Validate() error {
	if strings.TrimSpace(recipe.Name) == "" {
		return errors.New("recipe name is empty")
	}
	if strings.TrimSpace(recipe.Body) == "" {
		return fmt.Errorf("recipe %s body is empty", recipe.Name)
	}
	if strings.TrimSpace(recipe.Source) == "" {
		return fmt.Errorf("recipe %s source is empty", recipe.Name)
	}
	return recipe.Scope.Validate()
}

// Expand applies the runtime's documented client-side recipe substitution.
// $ARGUMENTS receives the trimmed input and $1..$9 receive whitespace-delimited
// arguments. A token such as $10 stays literal.
func (recipe Recipe) Expand(arguments string) (string, error) {
	if err := recipe.Validate(); err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(arguments)
	parts := strings.Fields(trimmed)
	return expandRecipeTemplate(recipe.Body, trimmed, parts), nil
}

func expandRecipeTemplate(template, allArguments string, positional []string) string {
	var expanded strings.Builder
	expanded.Grow(len(template))
	for offset := 0; offset < len(template); {
		if strings.HasPrefix(template[offset:], "$ARGUMENTS") {
			expanded.WriteString(allArguments)
			offset += len("$ARGUMENTS")
			continue
		}
		if template[offset] == '$' && offset+1 < len(template) && template[offset+1] >= '1' && template[offset+1] <= '9' &&
			(offset+2 == len(template) || template[offset+2] < '0' || template[offset+2] > '9') {
			index := int(template[offset+1] - '1')
			if index < len(positional) {
				expanded.WriteString(positional[index])
			}
			offset += 2
			continue
		}
		expanded.WriteByte(template[offset])
		offset++
	}
	return expanded.String()
}

type Service interface {
	Documents(context.Context, string) ([]Document, error)
	Recipes(context.Context, string) ([]Recipe, error)
}
