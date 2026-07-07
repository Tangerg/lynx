package server

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/lifecycle"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

type turnAccess interface {
	PlanTurnStart(ctx context.Context, sessionID, defaultCwd string, draft turn.StartTurnRequest) (sessionsvc.Session, turn.StartTurnRequest, error)
	StartTurn(ctx context.Context, req turn.StartTurnRequest) (turn.TurnHandle, error)
	TurnEvents(ctx context.Context, handle turn.TurnHandle) (iter.Seq[turn.Event], error)
	InjectTurnSteering(ctx context.Context, handle turn.TurnHandle, message string) error
	ResumeTurn(ctx context.Context, handle turn.TurnHandle, resolution interrupts.Resolution) error
	RehydrateTurn(ctx context.Context, req turn.RehydrateRequest) (turn.TurnHandle, error)
	CancelTurn(ctx context.Context, handle turn.TurnHandle) error
	TurnProcessID(ctx context.Context, handle turn.TurnHandle) (string, error)
	SetTurnInterruptKinds(kinds []string)
}

type lifecycleAccess interface {
	ClaimRunSlot(ctx context.Context, claims lifecycle.SessionClaimer, sessionID string) (lifecycle.RunAdmission, error)
	ClaimMutationSlot(claims lifecycle.SessionClaimer, sessionID string) (lifecycle.RunAdmission, error)
	ClaimWorkingTreeRun(cwd string) (lifecycle.WorkingTreeAdmission, bool)
	ClaimWorkingTreeMutation(cwd string) (lifecycle.WorkingTreeAdmission, bool)
	ClaimResumeSlot(ctx context.Context, claims lifecycle.SessionClaimer, parentRunID string) (interrupts.Pending, lifecycle.RunAdmission, error)
	CancelParkedRun(ctx context.Context, runID string) error
	CancelRunBinding(ctx context.Context, run lifecycle.RunTurnBinding) error
	ResumeClaimedInterrupt(ctx context.Context, parentRunID string, resolution interrupts.Resolution) (lifecycle.ResumedInterrupt, error)
	RollbackResolved(ctx context.Context, sessionID string, boundary lifecycle.RollbackBoundary) error
	ForkSession(ctx context.Context, spec lifecycle.ForkSpec) (sessionsvc.Session, error)
	RestoreSession(ctx context.Context, ses sessionsvc.Session, msgs []chat.Message, runs []transcript.Run, items []transcript.Item) error
}

type runSegmentAccess interface {
	RunSegmentEffects(checkpoints runsegment.Checkpoints, publish runsegment.FileChangePublisher) *runsegment.Effects
}

type interruptQueryAccess interface {
	ListPendingInterrupts(ctx context.Context, sessionID string) ([]interrupts.Pending, error)
}

type maintenanceAccess interface {
	GenerateTitle(ctx context.Context, firstMessage string) (string, error)
}
