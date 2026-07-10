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

// turnUseCases owns the lifecycle of an agent turn and the session mutations
// that must be coordinated with that lifecycle (resume, rollback, fork, and
// restore). Keeping the methods together makes the concurrency boundary
// explicit instead of scattering it across one-method interfaces.
type turnUseCases interface {
	PlanTurnStart(ctx context.Context, sessionID, defaultCwd string, draft turn.StartTurnRequest) (sessionsvc.Session, turn.StartTurnRequest, error)
	StartTurn(ctx context.Context, req turn.StartTurnRequest) (turn.TurnHandle, error)
	TurnEvents(ctx context.Context, handle turn.TurnHandle) (iter.Seq[turn.Event], error)
	CancelTurn(ctx context.Context, handle turn.TurnHandle) error
	InjectTurnSteering(ctx context.Context, handle turn.TurnHandle, message string) error
	ClaimRunSlot(ctx context.Context, claims lifecycle.SessionClaimer, sessionID string) (lifecycle.RunAdmission, error)
	ClaimWorkingTreeRun(cwd string) (lifecycle.WorkingTreeAdmission, bool)
	ClaimMutationSlot(claims lifecycle.SessionClaimer, sessionID string) (lifecycle.RunAdmission, error)
	ClaimWorkingTreeMutation(cwd string) (lifecycle.WorkingTreeAdmission, bool)
	ClaimResumeSlot(ctx context.Context, claims lifecycle.SessionClaimer, parentRunID string) (interrupts.Pending, lifecycle.RunAdmission, error)
	ResumeClaimedInterrupt(ctx context.Context, parentRunID string, resolution interrupts.Resolution, interruptKinds []string) (lifecycle.ResumedInterrupt, error)
	CancelParkedRun(ctx context.Context, runID string) error
	CancelRunBinding(ctx context.Context, run lifecycle.RunTurnBinding) error
	RollbackResolved(ctx context.Context, sessionID string, boundary transcript.Boundary) error
	ForkSession(ctx context.Context, spec lifecycle.ForkSpec) (sessionsvc.Session, error)
	RestoreSession(ctx context.Context, ses sessionsvc.Session, msgs []chat.Message, runs []transcript.Run, items []transcript.Item) error
	RunSegmentEffects(checkpoints runsegment.Checkpoints, publish runsegment.FileChangePublisher) *runsegment.Effects
	ListPendingInterrupts(ctx context.Context, sessionID string) ([]interrupts.Pending, error)
	GenerateTitle(ctx context.Context, firstMessage string) (string, error)
}
