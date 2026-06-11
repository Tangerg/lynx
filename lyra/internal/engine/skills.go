package engine

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"
	skillstool "github.com/Tangerg/lynx/tools/skills"

	"github.com/Tangerg/lynx/lyra/internal/service/skills"
)

// SkillInfo is re-exported from the skills service so the engine's accessor
// (and the runtime SPI / wire layer above it) name one type. Discovery and
// the project-over-global precedence live in [skills]; the engine keeps only
// the per-session-cwd resolution and the chat.Tool construction.
type SkillInfo = skills.Info

// buildSkillTool assembles the working-directory-scoped skill tool over the
// merged skill source (project <workdir>/.lyra/skills layered over the global
// dir, project winning). It returns nil when neither directory exists, so a
// session that ships no skills gets no skill tool at all.
//
// Rebuilt per resolution like fs/bash, because the project directory depends
// on the turn's working directory; the merged source just wraps os.DirFS, so
// the cost is negligible.
func buildSkillTool(workdir, globalDir string) chat.Tool {
	source := skills.MergedSource(skills.ProjectDir(workdir), globalDir)
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

// ListSkills enumerates the skills visible from workdir for the client —
// project skills layered over the global directory (the same precedence
// buildSkillTool gives the model). The engine resolves the two directories
// from the turn's working directory + its configured global dir; [skills.List]
// does the discovery, merge, and sort.
func (e *Engine) ListSkills(ctx context.Context, workdir string) ([]SkillInfo, error) {
	return skills.List(ctx, skills.ProjectDir(workdir), e.skillsGlobalDir)
}
