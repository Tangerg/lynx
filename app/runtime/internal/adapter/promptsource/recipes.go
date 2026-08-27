package promptsource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	workspaceapp "github.com/Tangerg/scope/app/runtime/internal/application/workspace"
)

const (
	recipeProjectSubdir = ".lyra/recipes"
	recipeFileExt       = ".md"
	// A recipes directory is an authored capability root rather than a generic
	// file browser. This adapter limit bounds scanning even when most entries do
	// not have the recipe extension.
	maxRecipeDirectoryEntries = 1024
)

// listRecipes enumerates recipe files from projectDir layered over globalDir,
// with the project copy winning on name collisions. This adapter owns the
// directory convention and the Markdown/YAML format; malformed frontmatter is
// preserved as a plain prompt rather than discarding user-authored content.
func listRecipes(ctx context.Context, projectDir, globalDir string) ([]workspaceapp.Recipe, error) {
	seen := make(map[string]struct{})
	var out []workspaceapp.Recipe
	totalBytes := 0
	add := func(dir string, scope workspaceapp.RecipeScope) error {
		entries, err := readRecipeDirectory(ctx, dir)
		if err != nil {
			return err
		}
		scopeCount := 0
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() {
				continue
			}
			name, ok := recipeName(entry.Name())
			if !ok {
				continue
			}
			scopeCount++
			if scopeCount > workspaceapp.MaxRecipesPerScope {
				return fmt.Errorf(
					"%w: recipe scope %q has more than %d recipes",
					workspaceapp.ErrPromptSourceTooLarge,
					scope,
					workspaceapp.MaxRecipesPerScope,
				)
			}
			if _, dup := seen[name]; dup {
				continue // a higher-precedence (project) source already provided it
			}
			path := filepath.Join(dir, entry.Name())
			data, err := readAuthoredPromptFile(ctx, path)
			if err != nil {
				return fmt.Errorf("promptsource: read recipe %q: %w", name, err)
			}
			if len(out) >= workspaceapp.MaxRecipeCascade {
				return fmt.Errorf(
					"%w: recipe cascade has more than %d recipes",
					workspaceapp.ErrPromptSourceTooLarge,
					workspaceapp.MaxRecipeCascade,
				)
			}
			if len(data) > workspaceapp.MaxRecipeCascadeBytes-totalBytes {
				return fmt.Errorf(
					"%w: recipe cascade exceeds %d bytes",
					workspaceapp.ErrPromptSourceTooLarge,
					workspaceapp.MaxRecipeCascadeBytes,
				)
			}
			seen[name] = struct{}{}
			totalBytes += len(data)
			out = append(out, parseRecipe(name, scope, path, data))
		}
		return nil
	}
	if err := add(projectDir, workspaceapp.RecipeScopeProject); err != nil {
		return nil, err
	}
	if err := add(globalDir, workspaceapp.RecipeScopeGlobal); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b workspaceapp.Recipe) int { return strings.Compare(a.Name, b.Name) })
	if err := workspaceapp.ValidateRecipeCascade(out); err != nil {
		return nil, err
	}
	return out, nil
}

func readRecipeDirectory(ctx context.Context, directory string) ([]os.DirEntry, error) {
	if directory == "" {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := os.Open(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("promptsource: open recipe directory %q: %w", directory, err)
	}
	defer dir.Close()
	entries, err := dir.ReadDir(maxRecipeDirectoryEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("promptsource: read recipe directory %q: %w", directory, err)
	}
	if len(entries) > maxRecipeDirectoryEntries {
		return nil, fmt.Errorf(
			"%w: recipe directory %q has more than %d entries",
			workspaceapp.ErrPromptSourceTooLarge,
			directory,
			maxRecipeDirectoryEntries,
		)
	}
	return entries, nil
}

func recipeDir(cwd string) string {
	if cwd == "" {
		return ""
	}
	return filepath.Join(cwd, recipeProjectSubdir)
}

func recipeName(filename string) (string, bool) {
	if strings.HasPrefix(filename, ".") || !strings.HasSuffix(filename, recipeFileExt) {
		return "", false
	}
	return strings.TrimSuffix(filename, recipeFileExt), true
}

type recipeFrontmatter struct {
	Description  string `yaml:"description"`
	ArgumentHint string `yaml:"argumentHint"`
}

func parseRecipe(name string, scope workspaceapp.RecipeScope, source string, content []byte) workspaceapp.Recipe {
	frontmatter, body := parseRecipeBody(content)
	return workspaceapp.Recipe{
		Name:         name,
		Description:  strings.TrimSpace(frontmatter.Description),
		ArgumentHint: strings.TrimSpace(frontmatter.ArgumentHint),
		Body:         body,
		Scope:        scope,
		Source:       source,
	}
}

func parseRecipeBody(content []byte) (recipeFrontmatter, string) {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	text = strings.TrimPrefix(text, "\ufeff")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return recipeFrontmatter{}, strings.TrimSpace(text)
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return recipeFrontmatter{}, strings.TrimSpace(text)
	}
	var frontmatter recipeFrontmatter
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &frontmatter); err != nil {
		return recipeFrontmatter{}, strings.TrimSpace(text)
	}
	return frontmatter, strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
}
