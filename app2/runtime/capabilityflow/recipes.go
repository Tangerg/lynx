package capabilityflow

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const maxRecipeNameBytes = 64

type recipeFrontmatter struct {
	Description  string `yaml:"description"`
	ArgumentHint string `yaml:"argumentHint"`
}

func (service *Service) Recipes(
	ctx context.Context,
	query protocol.WorkspaceQuery,
) (*protocol.Page[protocol.Recipe], error) {
	resolved, err := service.resolve(ctx, &query.Workspace)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	values := make([]protocol.Recipe, 0)
	project, err := readRecipeDirectory(
		ctx,
		resolved.Workspace.Path(),
		".lyra/recipes",
		protocol.RecipeScopeProject,
		seen,
	)
	if err != nil {
		return nil, err
	}
	values = append(values, project...)
	global, err := readRecipeDirectory(
		ctx,
		service.home,
		".lyra/recipes",
		protocol.RecipeScopeGlobal,
		seen,
	)
	if err != nil {
		return nil, err
	}
	values = append(values, global...)
	slices.SortFunc(values, func(left, right protocol.Recipe) int {
		return strings.Compare(left.Name, right.Name)
	})
	return protocol.NewPage(values), nil
}

func readRecipeDirectory(
	ctx context.Context,
	anchor string,
	directory string,
	scope protocol.RecipeScope,
	seen map[string]struct{},
) ([]protocol.Recipe, error) {
	root, err := os.OpenRoot(anchor)
	if err != nil {
		return nil, fmt.Errorf("capabilityflow: open recipe root: %w", err)
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("capabilityflow: read recipe directory: %w", err)
	}
	values := make([]protocol.Recipe, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name, ok := recipeName(entry.Name())
		if entry.IsDir() || !ok {
			continue
		}
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		relative := filepath.Join(directory, entry.Name())
		data, err := readBoundedRootFile(root, relative)
		if err != nil {
			return nil, fmt.Errorf("capabilityflow: read recipe %s: %w", name, err)
		}
		frontmatter, body := parseRecipe(data)
		seen[name] = struct{}{}
		values = append(values, protocol.Recipe{
			Name: name, Description: frontmatter.Description,
			ArgumentHint: frontmatter.ArgumentHint, Body: body,
			Scope: scope, Source: filepath.Join(anchor, relative),
		})
	}
	return values, nil
}

func recipeName(filename string) (string, bool) {
	if strings.HasPrefix(filename, ".") || !strings.HasSuffix(filename, ".md") {
		return "", false
	}
	name := strings.TrimSuffix(filename, ".md")
	if len(name) == 0 || len(name) > maxRecipeNameBytes {
		return "", false
	}
	for index := range len(name) {
		character := name[index]
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' {
			continue
		}
		return "", false
	}
	return name, true
}

func readBoundedRootFile(root *os.Root, name string) ([]byte, error) {
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	return readBoundedAuthoredFile(file, info)
}

func parseRecipe(data []byte) (recipeFrontmatter, string) {
	text := strings.TrimPrefix(strings.ReplaceAll(string(data), "\r\n", "\n"), "\ufeff")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return recipeFrontmatter{}, strings.TrimSpace(text)
	}
	end := -1
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "---" {
			end = index
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
	frontmatter.Description = strings.TrimSpace(frontmatter.Description)
	frontmatter.ArgumentHint = strings.TrimSpace(frontmatter.ArgumentHint)
	return frontmatter, strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
}
