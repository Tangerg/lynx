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

type turnStartAccess interface {
	PlanTurnStart(ctx context.Context, sessionID, defaultCwd string, draft turn.StartTurnRequest) (sessionsvc.Session, turn.StartTurnRequest, error)
	StartTurn(ctx context.Context, req turn.StartTurnRequest) (turn.TurnHandle, error)
}

type turnStreamAccess interface {
	TurnEvents(ctx context.Context, handle turn.TurnHandle) (iter.Seq[turn.Event], error)
	CancelTurn(ctx context.Context, handle turn.TurnHandle) error
}

type turnSteeringAccess interface {
	InjectTurnSteering(ctx context.Context, handle turn.TurnHandle, message string) error
}

type turnInterruptPolicyAccess interface {
	SetTurnInterruptKinds(kinds []string)
}

type runSlotAdmissionAccess interface {
	ClaimRunSlot(ctx context.Context, claims lifecycle.SessionClaimer, sessionID string) (lifecycle.RunAdmission, error)
}

type workingTreeRunAdmissionAccess interface {
	ClaimWorkingTreeRun(cwd string) (lifecycle.WorkingTreeAdmission, bool)
}

type sessionMutationSlotAccess interface {
	ClaimMutationSlot(claims lifecycle.SessionClaimer, sessionID string) (lifecycle.RunAdmission, error)
}

type workingTreeMutationAccess interface {
	ClaimWorkingTreeMutation(cwd string) (lifecycle.WorkingTreeAdmission, bool)
}

type runResumeAccess interface {
	ClaimResumeSlot(ctx context.Context, claims lifecycle.SessionClaimer, parentRunID string) (interrupts.Pending, lifecycle.RunAdmission, error)
	ResumeClaimedInterrupt(ctx context.Context, parentRunID string, resolution interrupts.Resolution) (lifecycle.ResumedInterrupt, error)
}

type runCancellationAccess interface {
	CancelParkedRun(ctx context.Context, runID string) error
	CancelRunBinding(ctx context.Context, run lifecycle.RunTurnBinding) error
}

type sessionRollbackAccess interface {
	RollbackResolved(ctx context.Context, sessionID string, boundary lifecycle.RollbackBoundary) error
}

type sessionForkAccess interface {
	ForkSession(ctx context.Context, spec lifecycle.ForkSpec) (sessionsvc.Session, error)
}

type sessionRestoreAccess interface {
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
