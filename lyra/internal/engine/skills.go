package engine

import (
	"os"
	"path/filepath"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/skills"
	skillstool "github.com/Tangerg/lynx/tools/skills"
)

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

// dirExists reports whether path names an existing directory.
func dirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
