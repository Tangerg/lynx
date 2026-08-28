package runmaintenance

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/infra/exec"
)

// NewLiveStateSnapshotter adapts process-owned live shells to the compactor's
// reminder source. Durable Session state has its own per-model-call projection
// and deliberately does not pass through summary maintenance.
func NewLiveStateSnapshotter(shells *exec.Shells) LiveStateSnapshotter {
	if shells == nil {
		return nil
	}
	return func(_ context.Context, sessionID string) LiveStateSnapshot {
		var snap LiveStateSnapshot
		for _, sh := range shells.RunningForSession(sessionID) {
			snap.Shells = append(snap.Shells, RunningShell{ID: sh.ID, Command: sh.Command})
		}
		return snap
	}
}
