package runtime

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/lifecycle"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

func (r *Runtime) lifecycle() *lifecycle.Coordinator {
	return lifecycle.New(lifecycleStores{rt: r})
}

type lifecycleStores struct {
	rt *Runtime
}

func (s lifecycleStores) Session() lifecycle.SessionStore { return s.rt.session }

func (s lifecycleStores) Transcript() lifecycle.TranscriptStore { return s.rt.transcript }

func (s lifecycleStores) Interrupts() lifecycle.InterruptStore { return s.rt.interrupts }

func (s lifecycleStores) ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error) {
	return s.rt.ReadHistory(ctx, sessionID)
}

func (s lifecycleStores) TruncateMessages(ctx context.Context, sessionID string, keepN int) error {
	return s.rt.TruncateMessages(ctx, sessionID, keepN)
}

func (s lifecycleStores) SeedHistory(ctx context.Context, sessionID string, msgs []chat.Message) error {
	return s.rt.SeedHistory(ctx, sessionID, msgs)
}

func (s lifecycleStores) ForgetSession(sessionID string) {
	s.rt.forgetSession(sessionID)
}

func (s lifecycleStores) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return s.rt.runInTx(ctx, fn)
}

type lifecycleTurns struct {
	rt *Runtime
}

func (t lifecycleTurns) Cancel(ctx context.Context, handle turn.TurnHandle) error {
	return t.rt.CancelTurn(ctx, handle)
}

func (t lifecycleTurns) Resume(ctx context.Context, handle turn.TurnHandle, resolution interrupts.Resolution) error {
	return t.rt.ResumeTurn(ctx, handle, resolution)
}

func (t lifecycleTurns) Rehydrate(ctx context.Context, req turn.RehydrateRequest) (turn.TurnHandle, error) {
	return t.rt.RehydrateTurn(ctx, req)
}

// ClaimRunSlot reserves the single-writer session slot for a new run.
func (r *Runtime) ClaimRunSlot(ctx context.Context, claims lifecycle.SessionClaimer, sessionID string) (lifecycle.RunAdmission, error) {
	return r.lifecycle().ClaimRunSlot(ctx, claims, sessionID)
}

// ClaimMutationSlot reserves the single-writer session slot for a destructive mutation.
func (r *Runtime) ClaimMutationSlot(claims lifecycle.SessionClaimer, sessionID string) (lifecycle.RunAdmission, error) {
	return r.lifecycle().ClaimMutationSlot(claims, sessionID)
}

// ClaimWorkingTreeRun reserves a working tree while a run segment is being admitted.
func (r *Runtime) ClaimWorkingTreeRun(cwd string) (lifecycle.WorkingTreeAdmission, bool) {
	return r.workingTrees.ClaimRun(worktree.CanonicalCwd(cwd))
}

// ClaimWorkingTreeMutation reserves exclusive access for a destructive working-tree mutation.
func (r *Runtime) ClaimWorkingTreeMutation(cwd string) (lifecycle.WorkingTreeAdmission, bool) {
	return r.workingTrees.ClaimMutation(worktree.CanonicalCwd(cwd))
}

// ClaimResumeSlot reserves the interrupted session before its interrupt is consumed.
func (r *Runtime) ClaimResumeSlot(ctx context.Context, claims lifecycle.SessionClaimer, parentRunID string) (interrupts.Pending, lifecycle.RunAdmission, error) {
	return r.lifecycle().ClaimResumeSlot(ctx, claims, parentRunID)
}

// CancelParkedRun abandons a durable open interrupt and its parked turn.
func (r *Runtime) CancelParkedRun(ctx context.Context, runID string) error {
	return r.lifecycle().CancelParkedRun(ctx, lifecycleTurns{rt: r}, runID)
}

// CancelRunBinding tears down the turn bound to a run and drops any durable interrupt record.
func (r *Runtime) CancelRunBinding(ctx context.Context, run lifecycle.RunTurnBinding) error {
	return r.lifecycle().CancelRunBinding(ctx, lifecycleTurns{rt: r}, run)
}

// ResumeClaimedInterrupt consumes an open interrupt and resumes or rehydrates its turn.
func (r *Runtime) ResumeClaimedInterrupt(ctx context.Context, parentRunID string, resolution interrupts.Resolution) (lifecycle.ResumedInterrupt, error) {
	return r.lifecycle().ResumeClaimedInterrupt(ctx, lifecycleTurns{rt: r}, parentRunID, resolution)
}

// RollbackResolved executes a resolved rollback write-set.
func (r *Runtime) RollbackResolved(ctx context.Context, sessionID string, boundary lifecycle.RollbackBoundary) error {
	return r.lifecycle().RollbackResolved(ctx, lifecycleTurns{rt: r}, sessionID, boundary)
}

// ForkSession creates a child session from a resolved fork boundary.
func (r *Runtime) ForkSession(ctx context.Context, spec lifecycle.ForkSpec) (session.Session, error) {
	return r.lifecycle().Fork(ctx, spec)
}

// RestoreSession replaces a session and its transcript from a decoded artifact.
func (r *Runtime) RestoreSession(ctx context.Context, ses session.Session, messages []chat.Message, runs []transcript.Run, items []transcript.Item) error {
	return r.lifecycle().RestoreSession(ctx, ses, messages, runs, items)
}

// DeleteSession removes a session and cascades its runtime-scoped state.
func (r *Runtime) DeleteSession(ctx context.Context, sessionID string) error {
	return r.lifecycle().DeleteSession(ctx, lifecycleTurns{rt: r}, sessionID)
}
