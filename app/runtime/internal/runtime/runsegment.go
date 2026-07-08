package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

type runSegmentProcesses struct {
	rt *Runtime
}

func (p runSegmentProcesses) ProcessID(ctx context.Context, handle turn.TurnHandle) (string, error) {
	return p.rt.TurnProcessID(ctx, handle)
}

type runSegmentStores struct {
	rt *Runtime
}

func (s runSegmentStores) Interrupts() interrupts.Store { return s.rt.interrupts }

func (s runSegmentStores) Session() runsegment.SessionStore { return s.rt.session }

func (s runSegmentStores) Transcript() transcript.Store { return s.rt.transcript }

func (s runSegmentStores) MessageCount(ctx context.Context, sessionID string) (int, error) {
	return s.rt.MessageCount(ctx, sessionID)
}

func (s runSegmentStores) GenerateTitle(ctx context.Context, firstMessage string) (string, error) {
	return s.rt.GenerateTitle(ctx, firstMessage)
}

// RunSegmentEffects builds the delivery-neutral side-effect coordinator for a run segment.
func (r *Runtime) RunSegmentEffects(checkpoints runsegment.Checkpoints, publish runsegment.FileChangePublisher) *runsegment.Effects {
	return runsegment.New(runsegment.Config{
		Stores:             runSegmentStores{rt: r},
		Processes:          runSegmentProcesses{rt: r},
		Checkpoints:        checkpoints,
		PublishFileChanges: publish,
	})
}
