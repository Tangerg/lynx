package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

type skillCatalog interface {
	ListSkills(ctx context.Context, workdir string) ([]kernel.SkillInfo, error)
}

// ListSkills enumerates the skills visible from cwd (project over global) for
// workspace.listSkills. Delegates to the skill catalog port.
func (r *Runtime) ListSkills(ctx context.Context, cwd string) ([]kernel.SkillInfo, error) {
	return r.skillCatalog.ListSkills(ctx, cwd)
}
