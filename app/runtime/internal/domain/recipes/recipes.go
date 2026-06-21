// Package recipes is the recipe-discovery domain: it finds prompt recipes
// (*.md with optional YAML frontmatter) under a project's .lyra/recipes and the
// global recipes directory, merging them (project winning on a name collision).
//
// A recipe is a user-invoked, parameterized prompt template: the client expands
// the body's $ARGUMENTS / $1..$9 placeholders with the user's input and sends
// the result as a turn. Discovery mirrors skills (two flat directories, project
// over global) — recipes are user-facing prompt templates, the read-only sibling
// of the model-facing skill repo, not the nested cascade hooks/AGENTS.md use.
//
// Recipes are inert text: unlike lifecycle hooks they execute nothing, so there
// is no trust gate. A cloned repo's recipe only pre-fills a prompt the user
// picks and sees before sending; any tool the resulting turn calls still passes
// the approval gate.
package recipes

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProjectSubdir is where per-project recipes live, relative to the session's
// working directory — the .lyra/ convention shared with .lyra/skills. Project
// recipes take precedence over the global set.
const ProjectSubdir = ".lyra/recipes"

// fileExt marks a recipe file: each *.md directly under a recipes directory is
// one recipe, and the file name without it is the recipe name and the slash
// command the recipe is invoked by (review.md → /review).
const fileExt = ".md"

// Scope tags where a discovered recipe came from.
const (
	ScopeProject = "project" // <workdir>/.lyra/recipes
	ScopeGlobal  = "global"  // <LYRA_HOME>/recipes
)

// Recipe is one discovered recipe: its frontmatter metadata plus the Markdown
// prompt-template body, tagged with the scope it came from. Body carries the
// $ARGUMENTS / $1..$9 placeholders the client substitutes at invocation.
type Recipe struct {
	Name         string // file name without .md — the slash command (review → /review)
	Description  string // frontmatter: shown in the slash menu / command palette
	ArgumentHint string // frontmatter: placeholder hint for the slash autocomplete
	Body         string // the prompt template
	Scope        string // ScopeProject | ScopeGlobal
	Source       string // absolute path of the .md file
}

// frontmatter is the optional YAML metadata block at the head of a recipe file.
// Both fields are optional — a recipe with no frontmatter is a plain prompt
// template whose whole content is the body.
type frontmatter struct {
	Description  string `yaml:"description"`
	ArgumentHint string `yaml:"argumentHint"`
}

// List enumerates the recipes visible from projectDir layered over globalDir,
// the project copy winning on a name collision (the same precedence the skill
// source gives the model). A missing directory contributes nothing; a file that
// can't be read is skipped rather than failing the whole listing. Result is
// sorted by name.
func List(ctx context.Context, projectDir, globalDir string) ([]Recipe, error) {
	seen := make(map[string]struct{})
	var out []Recipe
	add := func(dir, scope string) error {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil // missing / unreadable dir contributes nothing
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			name, ok := recipeName(entry)
			if !ok {
				continue
			}
			if _, dup := seen[name]; dup {
				continue // a higher-precedence (project) source already provided it
			}
			r, ok := loadFile(filepath.Join(dir, entry.Name()), name, scope)
			if !ok {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, r)
		}
		return nil
	}
	if err := add(projectDir, ScopeProject); err != nil {
		return nil, err
	}
	if err := add(globalDir, ScopeGlobal); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b Recipe) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// ProjectDir resolves the project recipes directory for a session working
// directory. Empty workdir → empty (no project recipes).
func ProjectDir(workdir string) string {
	if workdir == "" {
		return ""
	}
	return filepath.Join(workdir, ProjectSubdir)
}

// recipeName returns a directory entry's recipe name (its file name minus the
// .md extension), or ok=false when the entry isn't a regular .md file (dirs,
// dotfiles, and other extensions are skipped).
func recipeName(entry os.DirEntry) (string, bool) {
	if entry.IsDir() {
		return "", false
	}
	name := entry.Name()
	if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, fileExt) {
		return "", false
	}
	return strings.TrimSuffix(name, fileExt), true
}

// loadFile reads and parses one recipe file. ok=false (skip) only for an
// unreadable file; frontmatter is optional, so a file with none — or with a
// malformed block — still yields a recipe whose whole content is the body.
func loadFile(path, name, scope string) (Recipe, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Recipe{}, false
	}
	fm, body := parse(data)
	return Recipe{
		Name:         name,
		Description:  strings.TrimSpace(fm.Description),
		ArgumentHint: strings.TrimSpace(fm.ArgumentHint),
		Body:         body,
		Scope:        scope,
		Source:       path,
	}, true
}

// parse splits a recipe document into its optional YAML frontmatter and the
// Markdown body. A leading "---" line opens a frontmatter block closed by the
// next "---"; the trimmed remainder is the body. Without an opening fence — or
// when the fence is unterminated or its YAML doesn't parse — the whole document
// is the body and the frontmatter is zero (a bare prompt is a valid recipe).
func parse(content []byte) (frontmatter, string) {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	text = strings.TrimPrefix(text, "\ufeff")
	lines := strings.Split(text, "\n")

	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return frontmatter{}, strings.TrimSpace(text)
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return frontmatter{}, strings.TrimSpace(text) // unterminated fence — plain body
	}
	var fm frontmatter
	block := strings.Join(lines[1:end], "\n")
	if err := yaml.Unmarshal([]byte(block), &fm); err != nil {
		return frontmatter{}, strings.TrimSpace(text) // malformed YAML — plain body
	}
	body := strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	return fm, body
}
