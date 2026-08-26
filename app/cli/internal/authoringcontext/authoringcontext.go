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

func (d DocumentScope) Validate() error {
	switch d {
	case DocumentWorkingDirectory, DocumentProjectRoot, DocumentHome:
		return nil
	default:
		return fmt.Errorf("agent document scope %q is invalid", d)
	}
}

type Document struct {
	Path  string
	Title string
	Scope DocumentScope
}

func (d Document) Validate() error {
	if strings.TrimSpace(d.Path) == "" {
		return errors.New("agent document path is empty")
	}
	return d.Scope.Validate()
}

type RecipeScope string

const (
	ProjectRecipe RecipeScope = "project"
	GlobalRecipe  RecipeScope = "global"
)

func (r RecipeScope) Validate() error {
	if r != ProjectRecipe && r != GlobalRecipe {
		return fmt.Errorf("recipe scope %q is invalid", r)
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

func (r Recipe) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("recipe name is empty")
	}
	if strings.TrimSpace(r.Body) == "" {
		return fmt.Errorf("recipe %s body is empty", r.Name)
	}
	if strings.TrimSpace(r.Source) == "" {
		return fmt.Errorf("recipe %s source is empty", r.Name)
	}
	return r.Scope.Validate()
}

// Expand applies the runtime's documented client-side recipe substitution.
// $ARGUMENTS receives the trimmed input and $1..$9 receive whitespace-delimited
// arguments. A token such as $10 stays literal.
func (r Recipe) Expand(arguments string) (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(arguments)
	parts := strings.Fields(trimmed)
	return expandRecipeTemplate(r.Body, trimmed, parts), nil
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
