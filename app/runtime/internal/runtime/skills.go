package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

// ListSkills enumerates the skills visible from cwd (project over global) for
// workspace.listSkills. Delegates to the engine, which owns skill sourcing.
func (r *Runtime) ListSkills(ctx context.Context, cwd string) ([]kernel.SkillInfo, error) {
	return r.engine.ListSkills(ctx, cwd)
}
