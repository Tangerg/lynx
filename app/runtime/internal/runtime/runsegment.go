package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/kernel/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

type runSegmentProcesses struct {
	rt *Runtime
}

func (p runSegmentProcesses) ProcessID(ctx context.Context, handle turn.TurnHandle) (string, error) {
	return p.rt.TurnProcessID(ctx, handle)
}

// RunSegmentEffects builds the delivery-neutral side-effect coordinator for a run segment.
func (r *Runtime) RunSegmentEffects(checkpoints runsegment.Checkpoints, publish runsegment.FileChangePublisher) *runsegment.Effects {
	return runsegment.New(runsegment.Config{
		Stores:             runtimeStores{rt: r},
		Processes:          runSegmentProcesses{rt: r},
		Checkpoints:        checkpoints,
		PublishFileChanges: publish,
	})
}
