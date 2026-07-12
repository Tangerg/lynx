package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
)

// turnUseCases is the residual facade surface the delivery layer still drives: the
// run-segment durable-effects factory (whose deps only the facade holds) and the
// terminal auto-titler run off a finished root run. Turn control (plan / start /
// steer) is the turn.Control adapter (see [Server.turnControl]); the run lifecycle
// is the run coordinator; the read projections are the queries coordinator.
type turnUseCases interface {
	RunSegmentEffects(checkpoints runsegment.Checkpoints, publish runsegment.FileChangePublisher) *runsegment.Effects
	GenerateTitle(ctx context.Context, firstMessage string) (string, error)
}
