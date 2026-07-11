package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
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

func (s runSegmentStores) Interrupts() runsegment.InterruptStore { return s.rt.interrupts }

func (s runSegmentStores) Session() runsegment.SessionStore { return s.rt.sessions }

func (s runSegmentStores) Transcript() runsegment.TranscriptStore { return s.rt.transcript }

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
		Tasks:              &r.tasks,
		PublishFileChanges: publish,
	})
}
