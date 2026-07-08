// Package skills is the skill-discovery domain: it finds Agent Skills
// (SKILL.md) under a project's .lyra/skills and the global skills
// directory, merges them (project winning on a name collision), and
// projects the result for the model (a merged skill source) and for
// clients (a sorted Info list).
//
// It owns the discovery + precedence rules. The kernel layer keeps the
// per-session-cwd resolution and the chat.Tool construction; it asks
// this service for the merged source / the listing.
package skills

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	sdk "github.com/Tangerg/lynx/skills"
)

// ProjectSubdir is where per-project skills live, relative to the session's
// working directory — the .lyra/ convention shared with .lyra/AGENTS.md.
// Project skills take precedence over the global set.
const ProjectSubdir = ".lyra/skills"

// Info is one discovered skill, tagged with the scope it came from — the
// client-facing projection (workspace.listSkills) of what the merged source
// exposes to the model (same sources, same precedence).
type Info struct {
	Name        string
	Description string
	Scope       string // "project" (<workdir>/.lyra/skills) | "global"
}

// MergedSource builds the merged skill source: projectDir layered over
// globalDir, the project copy winning on name collisions. Returns nil when
// neither directory exists, so a session that ships no skills gets no skill
// tool at all rather than one that always lists nothing.
//
// Building a source just wraps an os.DirFS, so this is cheap enough to call per
// tool resolution (the engine rebuilds the skill tool per turn cwd).
func MergedSource(projectDir, globalDir string) sdk.ResourceSource {
	var sources []sdk.ResourceSource
	if dirExists(projectDir) {
		sources = append(sources, sdk.Dir(projectDir))
	}
	if dirExists(globalDir) {
		sources = append(sources, sdk.Dir(globalDir))
	}
	if len(sources) == 0 {
		return nil
	}
	return sdk.Merge(sources...)
}

// List enumerates the skills visible from projectDir layered over globalDir,
// project winning on a name collision (the same precedence MergedSource gives
// the model). A missing directory contributes nothing rather than erroring.
// Result is sorted by name.
func List(ctx context.Context, projectDir, globalDir string) ([]Info, error) {
	seen := make(map[string]struct{})
	var out []Info
	add := func(dir, scope string) error {
		if !dirExists(dir) {
			return nil
		}
		summaries, err := sdk.Dir(dir).List(ctx)
		if err != nil {
			return err
		}
		for _, s := range summaries {
			if _, dup := seen[s.Name]; dup {
				continue // a higher-precedence (project) source already provided it
			}
			seen[s.Name] = struct{}{}
			out = append(out, Info{Name: s.Name, Description: s.Description, Scope: scope})
		}
		return nil
	}
	if err := add(projectDir, "project"); err != nil {
		return nil, err
	}
	if err := add(globalDir, "global"); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b Info) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// ProjectDir resolves the project skills directory for a session working
// directory.
func ProjectDir(workdir string) string {
	return filepath.Join(workdir, ProjectSubdir)
}

// dirExists reports whether path names an existing directory.
func dirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
