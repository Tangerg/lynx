// Package recipes is the recipe model: the Recipe value type, the frontmatter/
// body parse rule, and the project-over-global precedence (.lyra/recipes). The
// filesystem discovery — walking the recipe directories and reading files — is
// the promptsource adapter's job (internal/adapter/promptsource), not this
// package's.
//
// A recipe is a user-invoked, parameterized prompt template: the client expands
// the body's $ARGUMENTS / $1..$9 placeholders with the user's input and sends
// the result as a turn. Discovery mirrors skills (two flat directories, project
// over global) — recipes are user-facing prompt templates, the read-only sibling
// of the model-facing skill repo.
//
// Recipes are inert text: unlike lifecycle hooks they execute nothing, so there
// is no trust gate. A cloned repo's recipe only pre-fills a prompt the user picks
// and sees before sending; any tool the resulting turn calls still passes the
// approval gate.
package recipes

import (
	"path/filepath"
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

// ProjectDir resolves the project recipes directory for a session working
// directory. Empty workdir → empty (no project recipes).
func ProjectDir(workdir string) string {
	if workdir == "" {
		return ""
	}
	return filepath.Join(workdir, ProjectSubdir)
}

// RecipeName returns a directory entry file name's recipe name (its name minus
// the .md extension), or ok=false when the entry isn't a regular .md file
// (dotfiles and other extensions are skipped). The caller has already excluded
// directories.
func RecipeName(filename string) (string, bool) {
	if strings.HasPrefix(filename, ".") || !strings.HasSuffix(filename, fileExt) {
		return "", false
	}
	return strings.TrimSuffix(filename, fileExt), true
}

// Parse builds a Recipe from a recipe file's raw bytes: it splits the optional
// YAML frontmatter from the Markdown body (see [parse]) and tags the result with
// name / scope / source. Frontmatter is optional, so a file with none — or with a
// malformed block — still yields a recipe whose whole content is the body.
func Parse(name, scope, source string, content []byte) Recipe {
	fm, body := parse(content)
	return Recipe{
		Name:         name,
		Description:  strings.TrimSpace(fm.Description),
		ArgumentHint: strings.TrimSpace(fm.ArgumentHint),
		Body:         body,
		Scope:        scope,
		Source:       source,
	}
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
