package maintenance

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/exec"
)

// PlanReader is the compactor's read-only view of a session plan list.
type PlanReader interface {
	List(ctx context.Context, sessionID string) ([]plan.Step, error)
}

// NewLiveState adapts live shells and persisted plan to the compactor's
// reminder source. A plan-read failure omits plan rather than failing the
// compaction it decorates.
func NewLiveState(shells *exec.Shells, plans PlanReader) LiveStateFunc {
	if shells == nil && plans == nil {
		return nil
	}
	return func(ctx context.Context, sessionID string) LiveStateSnapshot {
		var snap LiveStateSnapshot
		if shells != nil {
			for _, sh := range shells.RunningForSession(sessionID) {
				snap.Shells = append(snap.Shells, RunningShell{ID: sh.ID, Command: sh.Command})
			}
		}
		if plans != nil {
			if steps, err := plans.List(ctx, sessionID); err == nil {
				for _, step := range steps {
					if step.Status == plan.StatusInProgress {
						snap.Plan = append(snap.Plan, step.Description)
					}
				}
			}
		}
		return snap
	}
}
