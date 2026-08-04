package turn

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

type subtreeTurnProcess struct {
	rootCanceled bool
	targets      []string
	plan         agentexec.WaitingSubtreeCancellationPlan
	planErr      error
	discardErr   error
	discarded    chan<- struct{}
}

func (*subtreeTurnProcess) ID() string { return "process_root" }

func (*subtreeTurnProcess) Await() agentexec.TurnCompletion {
	return agentexec.TurnCompletion{Status: core.StatusRunning}
}

func (process *subtreeTurnProcess) Cancel(context.Context) error {
	process.rootCanceled = true
	return nil
}

func (process *subtreeTurnProcess) CancelSubtree(_ context.Context, processID string) error {
	process.targets = append(process.targets, processID)
	return nil
}

func (*subtreeTurnProcess) Resume(context.Context, []agentexec.SuspensionAnswer) error {
	return nil
}

func (*subtreeTurnProcess) PendingSuspensions(context.Context) ([]agentexec.PendingSuspension, error) {
	return nil, nil
}

func (*subtreeTurnProcess) CaptureWaitingCheckpoint(context.Context) (agentexec.WaitingCheckpoint, error) {
	return testWaitingCheckpointValue(), nil
}

func (process *subtreeTurnProcess) Discard(context.Context) error {
	if process.discarded != nil {
		process.discarded <- struct{}{}
	}
	return process.discardErr
}

func (process *subtreeTurnProcess) PlanWaitingSubtreeCancellation(
	context.Context,
	string,
) (agentexec.WaitingSubtreeCancellationPlan, error) {
	return process.plan, process.planErr
}

func TestCancelSubtreeDoesNotClaimTheTurnLifecycle(t *testing.T) {
	process := &subtreeTurnProcess{}
	handle := Handle{SessionID: "session", TurnID: "turn"}
	state := newRunningTestState(t.Context(), handle, process)
	controller := &controller{
		turns:        map[string]*turnState{handle.TurnID: state},
		seenSessions: map[string]struct{}{},
	}

	if err := controller.CancelSubtree(t.Context(), handle, "process_child"); err != nil {
		t.Fatalf("CancelSubtree: %v", err)
	}
	if len(process.targets) != 1 || process.targets[0] != "process_child" {
		t.Fatalf("subtree calls = %v, want [process_child]", process.targets)
	}
	if process.rootCanceled {
		t.Fatal("subtree cancellation called root Cancel")
	}
	if _, err := controller.findTurn(handle.TurnID); err != nil {
		t.Fatalf("subtree cancellation released the owning turn: %v", err)
	}
	if state.released() {
		t.Fatal("subtree cancellation marked the owning turn released")
	}
}

type stubWaitingSubtreePlan struct {
	applyErr    error
	continueErr error
	pending     []agentexec.PendingSuspension
	applied     int
	continued   int
}

func (*stubWaitingSubtreePlan) CanceledProcessIDs() []string { return []string{"process_child"} }

func (plan *stubWaitingSubtreePlan) PendingSuspensions() []agentexec.PendingSuspension {
	return append([]agentexec.PendingSuspension(nil), plan.pending...)
}

func (*stubWaitingSubtreePlan) Checkpoint() execution.ExecutorCheckpoint {
	return testWaitingCheckpointValue().Checkpoint
}

func (plan *stubWaitingSubtreePlan) Apply(context.Context) error {
	plan.applied++
	return plan.applyErr
}

func (plan *stubWaitingSubtreePlan) Continue(context.Context) error {
	plan.continued++
	return plan.continueErr
}

func TestPrepareWaitingCancellationProjectsTypedBoundaryAndReleasesClaimOnAbort(t *testing.T) {
	plan := &stubWaitingSubtreePlan{
		pending: []agentexec.PendingSuspension{{
			ProcessID:    "process_sibling",
			SuspensionID: "suspension_sibling",
			Prompt: []byte(
				`{"kind":"question","question":{"toolName":"ask_user","arguments":"{}","fields":[{"prompt":"Continue?","header":"Continue"}]}}`,
			),
		}},
	}
	process := &subtreeTurnProcess{plan: plan}
	handle := Handle{SessionID: "session", TurnID: "turn"}
	state := newRunningTestState(t.Context(), handle, process)
	if !state.parkIfLive() {
		t.Fatal("test turn did not park")
	}
	controller := &controller{
		turns:        map[string]*turnState{handle.TurnID: state},
		seenSessions: map[string]struct{}{},
	}

	prepared, err := controller.PrepareWaitingSubtreeCancellation(
		t.Context(),
		handle,
		"process_child",
	)
	if err != nil {
		t.Fatalf("PrepareWaitingSubtreeCancellation: %v", err)
	}
	pending := prepared.PendingSuspensions
	if len(pending) != 1 ||
		pending[0].ProcessID != "process_sibling" ||
		pending[0].SuspensionID != "suspension_sibling" ||
		pending[0].Interrupt.Kind != execution.QuestionInterrupt {
		t.Fatalf("projected pending suspensions = %+v", pending)
	}
	prepared.Mutation.Abort()
	if !state.claimWaitingMutation() {
		t.Fatal("Abort did not release the parked-turn mutation claim")
	}
	state.abortWaitingMutation()
}

func TestPreparedWaitingCancellationContinuationFailureDoesNotReenterLifecycleLock(t *testing.T) {
	discardErr := errors.New("discard failed")
	discarded := make(chan struct{}, 2)
	process := &subtreeTurnProcess{discardErr: discardErr, discarded: discarded}
	handle := Handle{SessionID: "session", TurnID: "turn"}
	state := newRunningTestState(t.Context(), handle, process)
	if !state.parkIfLive() || !state.claimWaitingMutation() {
		t.Fatal("test turn did not enter waiting mutation phase")
	}
	controller := &controller{
		turns:        map[string]*turnState{handle.TurnID: state},
		seenSessions: map[string]struct{}{},
	}
	continueErr := errors.New("continue failed")
	plan := &stubWaitingSubtreePlan{continueErr: continueErr}
	prepared := &preparedWaitingSubtreeCancellation{
		controller: controller,
		state:      state,
		plan:       plan,
	}

	done := make(chan error, 1)
	go func() {
		done <- prepared.Commit(t.Context(), runs.WaitingSubtreeContinues)
	}()
	select {
	case err := <-done:
		if !errors.Is(err, continueErr) {
			t.Fatalf("Commit error = %v, want continuation failure", err)
		}
		if got := err.Error(); !strings.Contains(got, handle.TurnID) {
			t.Fatalf("Commit error = %q, want turn identity", got)
		}
	case <-time.After(time.Second):
		t.Fatal("Commit deadlocked while terminalizing a failed continuation")
	}
	if plan.applied != 1 || plan.continued != 1 {
		t.Fatalf(
			"runtime plan calls = apply:%d continue:%d, want 1/1",
			plan.applied,
			plan.continued,
		)
	}
	if !state.terminalized() {
		t.Fatal("failed continuation did not terminalize the turn")
	}
	select {
	case <-discarded:
	case <-time.After(time.Second):
		t.Fatal("failed continuation did not attempt process teardown")
	}
	if state.released() {
		t.Fatal("failed process teardown released the turn")
	}

	shutdownCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := controller.AwaitShutdown(shutdownCtx); !errors.Is(err, discardErr) {
		t.Fatalf("shutdown error = %v, want process teardown failure", err)
	}
	process.discardErr = nil
	if err := controller.AwaitShutdown(shutdownCtx); err != nil {
		t.Fatalf("retry shutdown after teardown recovery: %v", err)
	}
	if !state.released() {
		t.Fatal("successful teardown retry did not release the turn")
	}
}

func TestPreparedWaitingCancellationRuntimeApplyFailureReleasesClaimOnAbort(t *testing.T) {
	process := &subtreeTurnProcess{}
	handle := Handle{SessionID: "session", TurnID: "turn"}
	state := newRunningTestState(t.Context(), handle, process)
	if !state.parkIfLive() || !state.claimWaitingMutation() {
		t.Fatal("test turn did not enter waiting mutation phase")
	}
	controller := &controller{
		turns:        map[string]*turnState{handle.TurnID: state},
		seenSessions: map[string]struct{}{},
	}
	applyErr := errors.New("runtime apply failed")
	plan := &stubWaitingSubtreePlan{applyErr: applyErr}
	prepared := &preparedWaitingSubtreeCancellation{
		controller: controller,
		state:      state,
		plan:       plan,
	}

	err := prepared.Commit(t.Context(), runs.WaitingSubtreeRemainsInterrupted)
	if !errors.Is(err, applyErr) {
		t.Fatalf("Commit error = %v, want runtime cause", err)
	}
	if !strings.Contains(err.Error(), handle.TurnID) {
		t.Fatalf("Commit error = %q, want turn identity", err)
	}
	prepared.Abort()
	if plan.applied != 1 {
		t.Fatalf("runtime apply calls = %d, want 1", plan.applied)
	}
	if !state.claimWaitingMutation() {
		t.Fatal("runtime apply failure retained the Application waiting-mutation claim")
	}
	state.abortWaitingMutation()
}
