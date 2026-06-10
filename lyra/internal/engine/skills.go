package engine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/skills"
	skillstool "github.com/Tangerg/lynx/tools/skills"
)

// SkillInfo is one skill discovered for a working directory, tagged with the
// scope it came from. It is the client-facing projection (workspace.listSkills)
// of what buildSkillTool exposes to the model — same sources, same precedence.
type SkillInfo struct {
	Name        string
	Description string
	Scope       string // "project" (<workdir>/.lyra/skills) | "global"
}

// projectSkillsSubdir is where per-project skills live, relative to the
// session's working directory — the .lyra/ convention shared with
// .lyra/AGENTS.md. Project skills take precedence over the global set.
const projectSkillsSubdir = ".lyra/skills"

// buildSkillTool assembles the working-directory-scoped skill tool: project
// skills (<workdir>/.lyra/skills) layered over the global skills directory,
// the project copy winning on name collisions. It returns nil when neither
// directory exists, so a session that ships no skills gets no skill tool at
// all rather than one that always lists nothing.
//
// Like the filesystem/bash tools it is rebuilt per resolution, because the
// project directory depends on the turn's working directory; building a
// [skills.FS] just wraps an os.DirFS, so the cost is negligible.
func buildSkillTool(workdir, globalDir string) chat.Tool {
	var sources []skills.Source
	if projectDir := filepath.Join(workdir, projectSkillsSubdir); dirExists(projectDir) {
		sources = append(sources, skills.Dir(projectDir))
	}
	if dirExists(globalDir) {
		sources = append(sources, skills.Dir(globalDir))
	}
	if len(sources) == 0 {
		return nil
	}
	// sources is non-empty, so the merged source is non-nil and NewTool cannot
	// fail; the error is checked only to satisfy the signature.
	tool, err := skillstool.NewTool(skills.Merge(sources...))
	if err != nil {
		return nil
	}
	return tool
}

// ListSkills enumerates the skills visible from workdir for the client —
// project skills (<workdir>/.lyra/skills) layered over the global directory,
// project winning on a name collision (the same precedence buildSkillTool
// gives the model). A missing directory contributes nothing rather than
// erroring (skills.FS.List treats absent roots as empty). Result is sorted by
// name. Rebuilt per call like buildSkillTool — an os.DirFS wrap is cheap.
func (e *Engine) ListSkills(ctx context.Context, workdir string) ([]SkillInfo, error) {
	seen := make(map[string]struct{})
	var out []SkillInfo
	add := func(dir, scope string) error {
		if !dirExists(dir) {
			return nil
		}
		summaries, err := skills.Dir(dir).List(ctx)
		if err != nil {
			return err
		}
		for _, s := range summaries {
			if _, dup := seen[s.Name]; dup {
				continue // a higher-precedence (project) source already provided it
			}
			seen[s.Name] = struct{}{}
			out = append(out, SkillInfo{Name: s.Name, Description: s.Description, Scope: scope})
		}
		return nil
	}
	if err := add(filepath.Join(workdir, projectSkillsSubdir), "project"); err != nil {
		return nil, err
	}
	if err := add(e.skillsGlobalDir, "global"); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b SkillInfo) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// dirExists reports whether path names an existing directory.
func dirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
