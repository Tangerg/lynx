package agentexec

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

// ListSkills enumerates the skills visible from workdir for the client —
// project skills layered over the global directory (the same precedence
// skill.Build gives the model). The engine resolves the two directories
// from the turn's working directory + its configured global dir; [skills.List]
// does the discovery, merge, and sort.
func (e *Engine) ListSkills(ctx context.Context, workdir string) ([]skills.Info, error) {
	return skills.List(ctx, skills.ProjectDir(workdir), e.skillsGlobalDir)
}
