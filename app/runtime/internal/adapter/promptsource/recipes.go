package promptsource

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/recipes"
)

// ListRecipes enumerates the recipes visible from projectDir layered over
// globalDir, the project copy winning on a name collision (the same precedence
// the skill source gives the model). A missing directory contributes nothing; a
// file that can't be read is skipped rather than failing the whole listing.
// Result is sorted by name. Each file's frontmatter/body split is the domain's
// ([recipes.Parse]); this function only supplies the filesystem walk + reads.
func ListRecipes(ctx context.Context, projectDir, globalDir string) ([]recipes.Recipe, error) {
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
			name, ok := recipes.RecipeName(entry.Name())
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
			out = append(out, recipes.Parse(name, scope, path, data))
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
