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
	prepared     agentexec.PreparedWaitingSubtreeCancellation
	prepareErr   error
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

func (process *subtreeTurnProcess) Discard(context.Context) error {
	if process.discarded != nil {
		process.discarded <- struct{}{}
	}
	return process.discardErr
}

func (process *subtreeTurnProcess) PrepareWaitingSubtreeCancellation(
	context.Context,
	string,
) (agentexec.PreparedWaitingSubtreeCancellation, error) {
	return process.prepared, process.prepareErr
}

func TestCancelSubtreeDoesNotClaimTheTurnLifecycle(t *testing.T) {
	process := &subtreeTurnProcess{}
	handle := TurnHandle{SessionID: "session", TurnID: "turn"}
	state := newRunningTestState(t.Context(), handle, process)
	dispatcher := &memoryDispatcher{
		turns:        map[string]*turnState{handle.TurnID: state},
		seenSessions: map[string]struct{}{},
	}

	if err := dispatcher.CancelSubtree(t.Context(), handle, "process_child"); err != nil {
		t.Fatalf("CancelSubtree: %v", err)
	}
	if len(process.targets) != 1 || process.targets[0] != "process_child" {
		t.Fatalf("subtree calls = %v, want [process_child]", process.targets)
	}
	if process.rootCanceled {
		t.Fatal("subtree cancellation called root Cancel")
	}
	if _, err := dispatcher.findTurn(handle.TurnID); err != nil {
		t.Fatalf("subtree cancellation released the owning turn: %v", err)
	}
	if state.released() {
		t.Fatal("subtree cancellation marked the owning turn released")
	}
}

type preparedSubtreeMutation struct {
	commitErr   error
	continueErr error
	pending     []agentexec.PendingSuspension
	committed   int
	continued   int
	aborted     int
}

func (*preparedSubtreeMutation) CanceledProcessIDs() []string { return []string{"process_child"} }

func (mutation *preparedSubtreeMutation) PendingSuspensions() []agentexec.PendingSuspension {
	return append([]agentexec.PendingSuspension(nil), mutation.pending...)
}

func (*preparedSubtreeMutation) PersistCheckpoint(context.Context) error { return nil }

func (mutation *preparedSubtreeMutation) Commit(context.Context) error {
	mutation.committed++
	return mutation.commitErr
}

func (mutation *preparedSubtreeMutation) Continue(context.Context) error {
	mutation.continued++
	return mutation.continueErr
}

func (mutation *preparedSubtreeMutation) Abort() { mutation.aborted++ }

func TestPrepareWaitingCancellationProjectsTypedBoundaryAndReleasesClaimOnAbort(t *testing.T) {
	mutation := &preparedSubtreeMutation{
		pending: []agentexec.PendingSuspension{{
			ProcessID:    "process_sibling",
			SuspensionID: "suspension_sibling",
			Prompt: []byte(
				`{"kind":"question","question":{"toolName":"ask_user","arguments":"{}","questions":[{"question":"Continue?","header":"Continue"}]}}`,
			),
		}},
	}
	process := &subtreeTurnProcess{prepared: mutation}
	handle := TurnHandle{SessionID: "session", TurnID: "turn"}
	state := newRunningTestState(t.Context(), handle, process)
	if !state.parkIfLive() {
		t.Fatal("test turn did not park")
	}
	dispatcher := &memoryDispatcher{
		turns:        map[string]*turnState{handle.TurnID: state},
		seenSessions: map[string]struct{}{},
	}

	prepared, err := dispatcher.PrepareWaitingSubtreeCancellation(
		t.Context(),
		handle,
		"process_child",
	)
	if err != nil {
		t.Fatalf("PrepareWaitingSubtreeCancellation: %v", err)
	}
	pending := prepared.PendingSuspensions()
	if len(pending) != 1 ||
		pending[0].ProcessID != "process_sibling" ||
		pending[0].SuspensionID != "suspension_sibling" ||
		pending[0].Interrupt.Kind != execution.QuestionInterrupt {
		t.Fatalf("projected pending suspensions = %+v", pending)
	}
	prepared.Abort()
	if mutation.aborted != 1 {
		t.Fatalf("runtime aborts = %d, want 1", mutation.aborted)
	}
	if !state.claimWaitingMutation() {
		t.Fatal("Abort did not release the parked-turn mutation claim")
	}
	state.abortWaitingMutation()
}

func TestPreparedWaitingCancellationContinuationFailureDoesNotReenterLifecycleLock(t *testing.T) {
	discardErr := errors.New("discard failed")
	discarded := make(chan struct{}, 2)
	process := &subtreeTurnProcess{discardErr: discardErr, discarded: discarded}
	handle := TurnHandle{SessionID: "session", TurnID: "turn"}
	state := newRunningTestState(t.Context(), handle, process)
	if !state.parkIfLive() || !state.claimWaitingMutation() {
		t.Fatal("test turn did not enter waiting mutation phase")
	}
	dispatcher := &memoryDispatcher{
		turns:        map[string]*turnState{handle.TurnID: state},
		seenSessions: map[string]struct{}{},
	}
	continueErr := errors.New("continue failed")
	mutation := &preparedSubtreeMutation{continueErr: continueErr}
	prepared := &preparedWaitingSubtreeCancellation{
		dispatcher: dispatcher,
		state:      state,
		prepared:   mutation,
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
	if mutation.committed != 1 || mutation.continued != 1 {
		t.Fatalf(
			"runtime mutation calls = commit:%d continue:%d, want 1/1",
			mutation.committed,
			mutation.continued,
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
	if err := dispatcher.AwaitShutdown(shutdownCtx); !errors.Is(err, discardErr) {
		t.Fatalf("shutdown error = %v, want process teardown failure", err)
	}
	process.discardErr = nil
	if err := dispatcher.AwaitShutdown(shutdownCtx); err != nil {
		t.Fatalf("retry shutdown after teardown recovery: %v", err)
	}
	if !state.released() {
		t.Fatal("successful teardown retry did not release the turn")
	}
}

func TestPreparedWaitingCancellationRuntimeCommitFailureReleasesClaimOnAbort(t *testing.T) {
	process := &subtreeTurnProcess{}
	handle := TurnHandle{SessionID: "session", TurnID: "turn"}
	state := newRunningTestState(t.Context(), handle, process)
	if !state.parkIfLive() || !state.claimWaitingMutation() {
		t.Fatal("test turn did not enter waiting mutation phase")
	}
	dispatcher := &memoryDispatcher{
		turns:        map[string]*turnState{handle.TurnID: state},
		seenSessions: map[string]struct{}{},
	}
	commitErr := errors.New("runtime commit failed")
	mutation := &preparedSubtreeMutation{commitErr: commitErr}
	prepared := &preparedWaitingSubtreeCancellation{
		dispatcher: dispatcher,
		state:      state,
		prepared:   mutation,
	}

	err := prepared.Commit(t.Context(), runs.WaitingSubtreeRemainsInterrupted)
	if !errors.Is(err, commitErr) {
		t.Fatalf("Commit error = %v, want runtime cause", err)
	}
	if !strings.Contains(err.Error(), handle.TurnID) {
		t.Fatalf("Commit error = %q, want turn identity", err)
	}
	prepared.Abort()
	if mutation.committed != 1 || mutation.aborted != 1 {
		t.Fatalf(
			"runtime mutation calls = commit:%d abort:%d, want 1/1",
			mutation.committed,
			mutation.aborted,
		)
	}
	if !state.claimWaitingMutation() {
		t.Fatal("runtime commit failure retained the waiting mutation claim")
	}
	state.abortWaitingMutation()
}
