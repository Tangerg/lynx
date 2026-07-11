// Package skill provides the skill tool — progressive-disclosure access to the
// SKILL.md skills visible from a turn's working directory. One tool, one
// package. It is working-directory scoped, so it's rebuilt per resolution.
package skill

import (
	"github.com/Tangerg/lynx/core/model/chat"
	skillstool "github.com/Tangerg/lynx/tools/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/promptsource"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

// Build assembles the working-directory-scoped skill tool over the merged skill
// source (project <workdir>/.lyra/skills layered over the global dir, project
// winning). It returns nil when neither directory exists, so a session that
// ships no skills gets no skill tool at all.
//
// Rebuilt per resolution like fs/shell, because the project directory depends on
// the turn's working directory; the merged source just wraps os.DirFS, so the
// cost is negligible.
func Build(workdir, globalDir string) chat.Tool {
	source := promptsource.MergeSkillSource(skills.ProjectDir(workdir), globalDir)
	if source == nil {
		return nil
	}
	// source is non-nil, so NewTool cannot fail; the error is checked only to
	// satisfy the signature.
	tool, err := skillstool.NewTool(source)
	if err != nil {
		return nil
	}
	return tool
}
