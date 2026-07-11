package server

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// turnUseCases owns the lifecycle of an agent turn: planning a start against a
// session, dispatching it, streaming and steering the live turn, cancel, and the
// run-boundary maintenance the delivery layer drives off it. The session/run
// lifecycle write-sets and single-writer admission gates that must be coordinated
// with a turn live on the sessions coordinator (see [Server.sessions]), not here.
// Keeping the turn methods together makes the concurrency boundary explicit
// instead of scattering it across one-method interfaces.
type turnUseCases interface {
	PlanTurnStart(ctx context.Context, sessionID, defaultCwd string, draft turn.StartTurnRequest) (sessionsvc.Session, turn.StartTurnRequest, error)
	StartTurn(ctx context.Context, req turn.StartTurnRequest) (turn.TurnHandle, error)
	// TurnEvents / CancelTurn satisfy the application's engine-neutral
	// [runs.Executor] (opaque handle + event); the facade holds the concrete turn
	// handle behind them.
	TurnEvents(ctx context.Context, handle runs.Handle) (iter.Seq[runs.EngineEvent], error)
	CancelTurn(ctx context.Context, handle runs.Handle) error
	InjectTurnSteering(ctx context.Context, handle turn.TurnHandle, message string) error
	RunSegmentEffects(checkpoints runsegment.Checkpoints, publish runsegment.FileChangePublisher) *runsegment.Effects
	ListPendingInterrupts(ctx context.Context, sessionID string) ([]interrupts.Pending, error)
	GenerateTitle(ctx context.Context, firstMessage string) (string, error)
}
