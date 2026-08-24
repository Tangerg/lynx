package workspace

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// MaxAuthoredPromptDocumentBytes bounds one complete AGENTS.md or Recipe
	// document shared by filesystem discovery and its eventual consumer.
	MaxAuthoredPromptDocumentBytes = 1 << 20
	// MaxAgentDocumentsPerCascade and MaxAgentDocumentCascadeBytes bound the
	// complete root-to-leaf instruction set before a model-facing projection.
	MaxAgentDocumentsPerCascade  = 64
	MaxAgentDocumentCascadeBytes = 4 << 20
	// MaxRecipesPerScope, MaxRecipeCascade, and MaxRecipeCascadeBytes bound the
	// complete project-over-global catalog published to clients.
	MaxRecipesPerScope    = 128
	MaxRecipeCascade      = 256
	MaxRecipeCascadeBytes = 8 << 20
)

var (
	// ErrPromptSourceTooLarge reports an authored document or complete source
	// collection that cannot enter the management and execution projections.
	ErrPromptSourceTooLarge = errors.New("workspace: prompt source too large")
	// ErrInvalidPromptSource reports authored material that cannot be represented
	// losslessly by the Runtime's UTF-8 prompt and JSON contracts.
	ErrInvalidPromptSource = errors.New("workspace: invalid prompt source")
)

// ValidateAuthoredPromptDocumentSize lets filesystem readers reject an
// already-oversized file before allocating its contents.
func ValidateAuthoredPromptDocumentSize(size int64) error {
	if size < 0 {
		return fmt.Errorf("%w: document size cannot be negative", ErrInvalidPromptSource)
	}
	if size > MaxAuthoredPromptDocumentBytes {
		return fmt.Errorf(
			"%w: document uses %d bytes, maximum %d",
			ErrPromptSourceTooLarge,
			size,
			MaxAuthoredPromptDocumentBytes,
		)
	}
	return nil
}

// ValidateAuthoredPromptDocument checks the complete bytes after reading, so a
// file that grows after stat cannot cross the same admission boundary.
func ValidateAuthoredPromptDocument(document []byte) error {
	if err := ValidateAuthoredPromptDocumentSize(int64(len(document))); err != nil {
		return err
	}
	if !utf8.Valid(document) {
		return fmt.Errorf("%w: document must be valid UTF-8", ErrInvalidPromptSource)
	}
	return nil
}

// AgentDocScope identifies the layer that supplied an instruction document.
// It is determined while walking the cascade and retained with the discovered
// value so later use cases never reconstruct semantic scope from a path.
type AgentDocScope string

const (
	AgentDocScopeHome        AgentDocScope = "home"
	AgentDocScopeCWD         AgentDocScope = "cwd"
	AgentDocScopeProjectRoot AgentDocScope = "projectRoot"
)

// AgentDocFile is the content read from one discovered AGENTS.md source. It
// carries source identity, cascade provenance, and content; prompt rendering
// remains an independent concern.
type AgentDocFile struct {
	Path    string
	Content string
	Scope   AgentDocScope
}

// ValidateAgentDocumentCascade protects every producer of AgentDocFile values,
// including ones that do not use the normal filesystem discovery path.
func ValidateAgentDocumentCascade(files []AgentDocFile) error {
	if len(files) > MaxAgentDocumentsPerCascade {
		return fmt.Errorf(
			"%w: agent document cascade has %d documents, maximum %d",
			ErrPromptSourceTooLarge,
			len(files),
			MaxAgentDocumentsPerCascade,
		)
	}
	total := 0
	for index, file := range files {
		if strings.TrimSpace(file.Path) == "" || strings.TrimSpace(file.Content) == "" {
			return fmt.Errorf("%w: agent document %d is incomplete", ErrInvalidPromptSource, index)
		}
		if !utf8.ValidString(file.Path) {
			return fmt.Errorf("%w: agent document %d path must be valid UTF-8", ErrInvalidPromptSource, index)
		}
		switch file.Scope {
		case AgentDocScopeHome, AgentDocScopeProjectRoot, AgentDocScopeCWD:
		default:
			return fmt.Errorf("%w: agent document %q has unknown scope %q", ErrInvalidPromptSource, file.Path, file.Scope)
		}
		if err := validateAuthoredPromptString(file.Content); err != nil {
			return fmt.Errorf("agent document %q: %w", file.Path, err)
		}
		if len(file.Content) > MaxAgentDocumentCascadeBytes-total {
			return fmt.Errorf(
				"%w: agent document cascade exceeds %d bytes",
				ErrPromptSourceTooLarge,
				MaxAgentDocumentCascadeBytes,
			)
		}
		total += len(file.Content)
	}
	return nil
}

// RecipeScope identifies the source layer that supplied a recipe.
type RecipeScope string

const (
	RecipeScopeProject RecipeScope = "project"
	RecipeScopeGlobal  RecipeScope = "global"
)

// Recipe is a discovered prompt template. Source layout and frontmatter have
// already been resolved; placeholder expansion belongs to the consumer.
type Recipe struct {
	Name         string
	Description  string
	ArgumentHint string
	Body         string
	Scope        RecipeScope
	Source       string
}

// ValidateRecipeCascade protects the complete-list contract independently of
// the normal filesystem discovery path.
func ValidateRecipeCascade(recipes []Recipe) error {
	if len(recipes) > MaxRecipeCascade {
		return fmt.Errorf(
			"%w: recipe cascade has %d recipes, maximum %d",
			ErrPromptSourceTooLarge,
			len(recipes),
			MaxRecipeCascade,
		)
	}
	perScope := make(map[RecipeScope]int, 2)
	total := 0
	for index, recipe := range recipes {
		if strings.TrimSpace(recipe.Name) == "" || strings.TrimSpace(recipe.Source) == "" {
			return fmt.Errorf("%w: recipe %d is incomplete", ErrInvalidPromptSource, index)
		}
		switch recipe.Scope {
		case RecipeScopeProject, RecipeScopeGlobal:
		default:
			return fmt.Errorf("%w: recipe %q has unknown scope %q", ErrInvalidPromptSource, recipe.Name, recipe.Scope)
		}
		perScope[recipe.Scope]++
		if perScope[recipe.Scope] > MaxRecipesPerScope {
			return fmt.Errorf(
				"%w: recipe scope %q has more than %d recipes",
				ErrPromptSourceTooLarge,
				recipe.Scope,
				MaxRecipesPerScope,
			)
		}
		material := len(recipe.Description) + len(recipe.ArgumentHint) + len(recipe.Body)
		for _, value := range []string{recipe.Name, recipe.Description, recipe.ArgumentHint, recipe.Body, recipe.Source} {
			if !utf8.ValidString(value) {
				return fmt.Errorf("%w: recipe %q must be valid UTF-8", ErrInvalidPromptSource, recipe.Name)
			}
		}
		if material > MaxAuthoredPromptDocumentBytes {
			return fmt.Errorf("%w: recipe %q exceeds %d bytes", ErrPromptSourceTooLarge, recipe.Name, MaxAuthoredPromptDocumentBytes)
		}
		if material > MaxRecipeCascadeBytes-total {
			return fmt.Errorf(
				"%w: recipe cascade exceeds %d bytes",
				ErrPromptSourceTooLarge,
				MaxRecipeCascadeBytes,
			)
		}
		total += material
	}
	return nil
}

func validateAuthoredPromptString(document string) error {
	if len(document) > MaxAuthoredPromptDocumentBytes {
		return fmt.Errorf(
			"%w: document uses %d bytes, maximum %d",
			ErrPromptSourceTooLarge,
			len(document),
			MaxAuthoredPromptDocumentBytes,
		)
	}
	if !utf8.ValidString(document) {
		return fmt.Errorf("%w: document must be valid UTF-8", ErrInvalidPromptSource)
	}
	return nil
}

// SkillScope identifies the source layer selected by prompt-source precedence.
type SkillScope string

const (
	SkillScopeProject SkillScope = "project"
	SkillScopeUser    SkillScope = "user"
)

// SkillSummary is one skill visible to a workspace, including the source layer
// selected by prompt-source precedence.
type SkillSummary struct {
	Name        string
	Description string
	Scope       SkillScope
}
