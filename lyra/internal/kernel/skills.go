package kernel

import (
	"context"

	"github.com/Tangerg/lynx/lyra/internal/domain/skills"
)

// SkillInfo is re-exported from the skills service so the engine's accessor
// (and the runtime SPI / wire layer above it) name one type. Discovery and
// the project-over-global precedence live in [skills]; the chat.Tool
// construction lives in the toolset layer ([toolset.BuildSkillTool]).
type SkillInfo = skills.Info

// ListSkills enumerates the skills visible from workdir for the client —
// project skills layered over the global directory (the same precedence
// buildSkillTool gives the model). The engine resolves the two directories
// from the turn's working directory + its configured global dir; [skills.List]
// does the discovery, merge, and sort.
func (e *Engine) ListSkills(ctx context.Context, workdir string) ([]SkillInfo, error) {
	return skills.List(ctx, skills.ProjectDir(workdir), e.skillsGlobalDir)
}
