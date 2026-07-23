package promptsource

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/recipes"
)

const (
	recipeProjectSubdir = ".lyra/recipes"
	recipeFileExt       = ".md"
)

// listRecipes enumerates recipe files from projectDir layered over globalDir,
// with the project copy winning on name collisions. This adapter owns the
// directory convention and the Markdown/YAML format; malformed frontmatter is
// preserved as a plain prompt rather than discarding user-authored content.
func listRecipes(ctx context.Context, projectDir, globalDir string) ([]recipes.Recipe, error) {
	seen := make(map[string]struct{})
	var out []recipes.Recipe
	add := func(dir, scope string) error {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil // missing / unreadable dir contributes nothing
		}
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
			if _, dup := seen[name]; dup {
				continue // a higher-precedence (project) source already provided it
			}
			path := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, parseRecipe(name, scope, path, data))
		}
		return nil
	}
	if err := add(projectDir, recipes.ScopeProject); err != nil {
		return nil, err
	}
	if err := add(globalDir, recipes.ScopeGlobal); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b recipes.Recipe) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func recipeDir(workdir string) string {
	if workdir == "" {
		return ""
	}
	return filepath.Join(workdir, recipeProjectSubdir)
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

func parseRecipe(name, scope, source string, content []byte) recipes.Recipe {
	frontmatter, body := parseRecipeBody(content)
	return recipes.Recipe{
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
