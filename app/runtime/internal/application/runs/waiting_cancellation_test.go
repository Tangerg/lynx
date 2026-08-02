package runs

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/application/change"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

type fakeItemProjection struct {
	items map[string]transcript.Item
	err   error
}

func (projection *fakeItemProjection) Item(
	_ context.Context,
	itemID string,
) (transcript.Item, bool, error) {
	if projection.err != nil {
		return transcript.Item{}, false, projection.err
	}
	item, found := projection.items[itemID]
	return item, found, nil
}

type fakePreparedWaitingCancellation struct {
	canceled    []string
	suspensions []ProcessSuspension
	checkpoint  *execution.ExecutorCheckpoint
	commitErr   error
	// settleOnCommitError models the real turn adapter's post-commit Continue
	// failure: the planned runtime transition is already applied, so Abort must
	// be a no-op while the opened segment error-terminalizes.
	settleOnCommitError bool

	disposition WaitingSubtreeDisposition
	committed   int
	aborted     int
	settled     bool
}

func (prepared *fakePreparedWaitingCancellation) value() PreparedWaitingSubtreeCancellation {
	checkpoint := testExecutorCheckpoint()
	if prepared.checkpoint != nil {
		checkpoint = prepared.checkpoint.Clone()
	}
	return PreparedWaitingSubtreeCancellation{
		CanceledProcessIDs: slices.Clone(prepared.canceled),
		PendingSuspensions: slices.Clone(prepared.suspensions),
		Checkpoint:         checkpoint,
		Mutation:           prepared,
	}
}

func TestPrepareWaitingCancellationRejectsCheckpointBoundToDifferentApplicationFacts(t *testing.T) {
	plan := waitingCancellationPlan(t, "run_a", false)
	for name, mutate := range map[string]func(*execution.ExecutorCheckpoint){
		"root":       func(checkpoint *execution.ExecutorCheckpoint) { checkpoint.RootProcessID = "other_root" },
		"session":    func(checkpoint *execution.ExecutorCheckpoint) { checkpoint.Scope.SessionID = "other_session" },
		"goal lease": func(checkpoint *execution.ExecutorCheckpoint) { checkpoint.Scope.GoalLeaseID = "other_goal" },
		"provider": func(checkpoint *execution.ExecutorCheckpoint) {
			checkpoint.ModelSelection = mustSelection("anthropic", checkpoint.ModelSelection.Model())
		},
		"model": func(checkpoint *execution.ExecutorCheckpoint) {
			checkpoint.ModelSelection = mustSelection(checkpoint.ModelSelection.Provider(), "other-model")
		},
	} {
		t.Run(name, func(t *testing.T) {
			checkpoint := testExecutorCheckpoint()
			mutate(&checkpoint)
			prepared := &fakePreparedWaitingCancellation{
				canceled:   []string{"process_a", "process_grandchild"},
				checkpoint: &checkpoint,
			}
			_, err := prepareWaitingCancellationTransformation(
				plan,
				"stop delegated branch",
				time.Date(2026, 7, 30, 2, 3, 4, 0, time.UTC),
				prepared.value(),
			)
			if !errors.Is(err, execution.ErrInvalidExecutorCheckpoint) {
				t.Fatalf("prepare error = %v, want ErrInvalidExecutorCheckpoint", err)
			}
			if prepared.committed != 0 || prepared.aborted != 0 {
				t.Fatalf("mutation touched before ownership validation: committed=%d aborted=%d", prepared.committed, prepared.aborted)
			}
		})
	}
}

func (prepared *fakePreparedWaitingCancellation) Commit(
	_ context.Context,
	disposition WaitingSubtreeDisposition,
) error {
	prepared.committed++
	prepared.disposition = disposition
	if prepared.commitErr != nil {
		if prepared.settleOnCommitError {
			prepared.settled = true
		}
		return prepared.commitErr
	}
	prepared.settled = true
	return nil
}

func (prepared *fakePreparedWaitingCancellation) Abort() {
	if prepared.settled {
		return
	}
	prepared.aborted++
	prepared.settled = true
}

func TestPrepareWaitingCancellationKeepsSurvivingExternalBoundary(t *testing.T) {
	plan := waitingCancellationPlan(t, "run_a", false)
	prepared := &fakePreparedWaitingCancellation{
		canceled: []string{"process_a", "process_grandchild"},
		suspensions: []ProcessSuspension{{
			ProcessID:    "process_b",
			SuspensionID: "suspension_b",
			Interrupt:    waitingQuestionPrompt(),
		}},
	}
	finishedAt := time.Date(2026, 7, 30, 2, 3, 4, 0, time.UTC)

	transformation, err := prepareWaitingCancellationTransformation(
		plan,
		"stop delegated branch",
		finishedAt,
		prepared.value(),
	)
	if err != nil {
		t.Fatalf("prepare waiting cancellation: %v", err)
	}
	if got, want := runIDs(transformation.terminalRuns), []string{"run_grandchild", "run_a"}; !slices.Equal(got, want) {
		t.Fatalf("terminal Runs = %v, want canonical postorder %v", got, want)
	}
	if len(transformation.terminalItems) != 1 {
		t.Fatalf("terminal interrupt Items = %+v, want one", transformation.terminalItems)
	}
	terminalItem := transformation.terminalItems[0]
	if terminalItem.Expected.ID != "item_grandchild" ||
		terminalItem.Expected.Status != transcript.ItemRunning ||
		terminalItem.Replacement.Status != transcript.ItemIncomplete ||
		terminalItem.Replacement.Error != nil {
		t.Fatalf(
			"terminal interrupt Item = %+v, want Running question settled Incomplete",
			terminalItem,
		)
	}
	if transformation.remaining == nil ||
		len(transformation.remaining.Interrupts) != 1 ||
		transformation.remaining.Interrupts[0].RunID != "run_b" {
		t.Fatalf("remaining Pending = %+v, want only run_b boundary", transformation.remaining)
	}
	assertSettledParentTool(t, transformation, "item_spawn_a", "call_run_a")
	if got := continuationRunIDs(transformation.continuation.continuations); !slices.Equal(got, []string{"run_b", "run_1"}) {
		t.Fatalf("continuation Runs = %v, want [run_b run_1]", got)
	}
	for _, run := range transformation.terminalRuns {
		if run.State != execution.Canceled ||
			run.Outcome == nil ||
			*run.Outcome != execution.OutcomeCanceled ||
			run.Detail != "stop delegated branch" ||
			!run.FinishedAt.Equal(finishedAt) {
			t.Fatalf("terminal Run = %+v, want exact canceled snapshot", run)
		}
	}
}

func TestPrepareWaitingCancellationContinuesAfterFinalBoundaryIsRemoved(t *testing.T) {
	plan := waitingCancellationPlan(t, "run_a", true)
	prepared := &fakePreparedWaitingCancellation{
		canceled: []string{"process_a", "process_grandchild"},
	}

	transformation, err := prepareWaitingCancellationTransformation(
		plan,
		"stop final waiting branch",
		time.Date(2026, 7, 30, 2, 3, 4, 0, time.UTC),
		prepared.value(),
	)
	if err != nil {
		t.Fatalf("prepare waiting cancellation: %v", err)
	}
	if transformation.remaining != nil {
		t.Fatalf("remaining Pending = %+v, want none", transformation.remaining)
	}
	if len(transformation.continuation.interrupts) != 0 {
		t.Fatalf("continuation interrupts = %+v, want none", transformation.continuation.interrupts)
	}
	if got := continuationRunIDs(transformation.continuation.continuations); !slices.Equal(got, []string{"run_b", "run_1"}) {
		t.Fatalf("continuation Runs = %v, want [run_b run_1]", got)
	}
	assertSettledParentTool(t, transformation, "item_spawn_a", "call_run_a")
}

func TestPublishWaitingChildCancellationInvalidatesExactReadSet(t *testing.T) {
	tests := []struct {
		name           string
		finalBoundary  bool
		suspensions    []ProcessSuspension
		affectedRunIDs []string
	}{
		{
			name: "remaining waiting boundary",
			suspensions: []ProcessSuspension{{
				ProcessID:    "process_b",
				SuspensionID: "suspension_b",
				Interrupt:    waitingQuestionPrompt(),
			}},
			affectedRunIDs: []string{"run_grandchild", "run_a", "run_1"},
		},
		{
			name:           "final waiting boundary",
			finalBoundary:  true,
			affectedRunIDs: []string{"run_grandchild", "run_a", "run_1", "run_b"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := waitingCancellationPlan(t, "run_a", test.finalBoundary)
			prepared := &fakePreparedWaitingCancellation{
				canceled:    []string{"process_a", "process_grandchild"},
				suspensions: test.suspensions,
			}
			transformation, err := prepareWaitingCancellationTransformation(
				plan,
				"stop delegated branch",
				time.Date(2026, 7, 30, 2, 3, 4, 0, time.UTC),
				prepared.value(),
			)
			if err != nil {
				t.Fatalf("prepare waiting cancellation: %v", err)
			}
			changes := &changeRecorder{}
			coordinator := &Coordinator{changed: changes.publish}

			coordinator.publishWaitingChildCancellation(plan, transformation)

			want := []change.Notice{
				change.InSession(change.Runs, plan.pending.SessionID, test.affectedRunIDs...),
				change.InSession(change.Interrupts, plan.pending.SessionID, plan.root.run.ID),
				change.InSession(change.Sessions, plan.pending.SessionID),
			}
			if got := changes.snapshot(); !reflect.DeepEqual(got, want) {
				t.Fatalf("published notices = %+v, want %+v", got, want)
			}
		})
	}
}

func TestCancelWaitingChildCommitsReducedPendingBeforeRuntimeTransition(t *testing.T) {
	plan := waitingCancellationPlan(t, "run_a", false)
	prepared := &fakePreparedWaitingCancellation{
		canceled: []string{"process_a", "process_grandchild"},
		suspensions: []ProcessSuspension{{
			ProcessID:    "process_b",
			SuspensionID: "suspension_b",
			Interrupt:    waitingQuestionPrompt(),
		}},
	}
	effects := &fakeEffects{
		waitingResult: WaitingSubtreeCancellationResult{
			TargetRun: canceledWaitingRun(
				plan.target.run,
				"stop delegated branch",
				time.Date(2026, 7, 30, 2, 3, 4, 0, time.UTC),
			),
			RootRun: plan.root.run,
		},
	}
	coordinator, turns := waitingCancellationCoordinator(t, plan, prepared, effects, &fakeExecutor{})

	result, err := coordinator.Cancel(t.Context(), CancelCommand{
		RunID:         plan.target.run.ID,
		Reason:        "stop delegated branch",
		AllowChildRun: true,
	})
	if err != nil {
		t.Fatalf("Cancel waiting child: %v", err)
	}
	if result.Run.ID != plan.target.run.ID ||
		result.RootRun == nil ||
		result.RootRun.State != execution.Interrupted {
		t.Fatalf("Cancel result = %+v, want canceled child and interrupted root", result)
	}
	if prepared.committed != 1 ||
		prepared.disposition != WaitingSubtreeRemainsInterrupted ||
		prepared.aborted != 0 {
		t.Fatalf(
			"prepared settlement = commits:%d disposition:%d aborts:%d",
			prepared.committed,
			prepared.disposition,
			prepared.aborted,
		)
	}
	if len(effects.waitingCancels) != 1 || effects.waitingCancels[0].RemainingPending == nil {
		t.Fatalf("durable waiting commits = %+v, want one reduced Pending", effects.waitingCancels)
	}
	if turns.prepared != plan.turn {
		t.Fatalf("prepared turn = %+v, want %+v", turns.prepared, plan.turn)
	}
}

func TestCancelWaitingChildRecoversCommittedTreeWhenRuntimeApplyFails(t *testing.T) {
	plan := waitingCancellationPlan(t, "run_a", false)
	applyErr := errors.New("runtime apply failed")
	prepared := &fakePreparedWaitingCancellation{
		canceled:  []string{"process_a", "process_grandchild"},
		commitErr: applyErr,
		suspensions: []ProcessSuspension{{
			ProcessID:    "process_b",
			SuspensionID: "suspension_b",
			Interrupt:    waitingQuestionPrompt(),
		}},
	}
	effects := &fakeEffects{
		waitingResult: WaitingSubtreeCancellationResult{
			TargetRun: canceledWaitingRun(
				plan.target.run,
				"stop delegated branch",
				time.Date(2026, 7, 30, 2, 3, 4, 0, time.UTC),
			),
			RootRun: plan.root.run,
		},
	}
	coordinator, turns := waitingCancellationCoordinator(t, plan, prepared, effects, &fakeExecutor{})
	sessions := coordinator.sessions.(*fakeRunSessions)
	var operations []string
	sessions.operations = &operations
	turns.operations = &operations

	_, err := coordinator.Cancel(t.Context(), CancelCommand{
		RunID:         plan.target.run.ID,
		Reason:        "stop delegated branch",
		AllowChildRun: true,
	})
	if !errors.Is(err, applyErr) {
		t.Fatalf("Cancel error = %v, want runtime apply failure", err)
	}
	if prepared.committed != 1 || prepared.aborted != 1 {
		t.Fatalf(
			"prepared settlement = commits:%d aborts:%d, want 1/1",
			prepared.committed,
			prepared.aborted,
		)
	}
	if sessions.lostRunID != plan.root.run.ID {
		t.Fatalf("recovered Run = %q, want root %q", sessions.lostRunID, plan.root.run.ID)
	}
	if !reflect.DeepEqual(turns.canceled, []execution.TurnRef{plan.turn}) {
		t.Fatalf("canceled turns = %+v, want [%+v]", turns.canceled, plan.turn)
	}
	if !slices.Equal(operations, []string{"durable.lost", "turn.cancel"}) {
		t.Fatalf("recovery operations = %v, want durable.lost then turn.cancel", operations)
	}
}

func TestCancelWaitingChildRehydratesParkedTurnAfterProcessRestart(t *testing.T) {
	plan := waitingCancellationPlan(t, "run_a", false)
	prepared := &fakePreparedWaitingCancellation{
		canceled: []string{"process_a", "process_grandchild"},
		suspensions: []ProcessSuspension{{
			ProcessID:    "process_b",
			SuspensionID: "suspension_b",
			Interrupt:    waitingQuestionPrompt(),
		}},
	}
	effects := &fakeEffects{
		waitingResult: WaitingSubtreeCancellationResult{
			TargetRun: canceledWaitingRun(
				plan.target.run,
				"stop after restart",
				time.Date(2026, 7, 30, 2, 3, 4, 0, time.UTC),
			),
			RootRun: plan.root.run,
		},
	}
	coordinator, turns := waitingCancellationCoordinator(t, plan, prepared, effects, &fakeExecutor{})
	turns.prepared = execution.TurnRef{}
	turns.prepareErr = ErrTurnNotLive
	turns.rehydrated = plan.turn

	if _, err := coordinator.Cancel(t.Context(), CancelCommand{
		RunID:         plan.target.run.ID,
		Reason:        "stop after restart",
		AllowChildRun: true,
	}); err != nil {
		t.Fatalf("Cancel rehydrated waiting child: %v", err)
	}
	rootContinuation, _ := plan.pending.RootContinuation()
	if turns.rehydrateReq.SessionID != plan.pending.SessionID ||
		turns.rehydrateReq.TurnID != plan.pending.TurnID ||
		turns.rehydrateReq.ProcessID != rootContinuation.ProcessID ||
		turns.rehydrateReq.Cwd != "/work" ||
		turns.rehydrateReq.ModelSelection != rootContinuation.ModelSelection {
		t.Fatalf("rehydrate request = %+v, want durable root continuation", turns.rehydrateReq)
	}
	if prepared.committed != 1 ||
		prepared.disposition != WaitingSubtreeRemainsInterrupted {
		t.Fatalf(
			"rehydrated prepared settlement = commits:%d disposition:%d",
			prepared.committed,
			prepared.disposition,
		)
	}
}

func TestCancelWaitingChildOpensContinuationWhenFinalBoundaryIsRemoved(t *testing.T) {
	plan := waitingCancellationPlan(t, "run_a", true)
	prepared := &fakePreparedWaitingCancellation{
		canceled: []string{"process_a", "process_grandchild"},
	}
	rootAfterCommit := plan.root.run
	rootAfterCommit.State = execution.Running
	rootAfterCommit.ActiveSegmentID = "seg_root_continuation"
	rootAfterCommit.Interrupts = nil
	effects := &fakeEffects{
		waitingResult: WaitingSubtreeCancellationResult{
			TargetRun: canceledWaitingRun(
				plan.target.run,
				"stop final waiting branch",
				time.Date(2026, 7, 30, 2, 3, 4, 0, time.UTC),
			),
			RootRun: rootAfterCommit,
		},
	}
	executor := &fakeExecutor{block: true}
	coordinator, _ := waitingCancellationCoordinator(t, plan, prepared, effects, executor)
	segmentIDs := []string{"seg_root_continuation", "seg_b_continuation"}
	coordinator.newSegmentID = func() string {
		if len(segmentIDs) == 0 {
			t.Fatal("unexpected extra segment identity")
		}
		next := segmentIDs[0]
		segmentIDs = segmentIDs[1:]
		return next
	}
	t.Cleanup(func() {
		coordinator.BeginShutdown()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := coordinator.AwaitShutdown(shutdownCtx); err != nil {
			t.Errorf("await Coordinator shutdown: %v", err)
		}
	})

	result, err := coordinator.Cancel(t.Context(), CancelCommand{
		RunID:         plan.target.run.ID,
		Reason:        "stop final waiting branch",
		AllowChildRun: true,
	})
	if err != nil {
		t.Fatalf("Cancel final waiting child: %v", err)
	}
	if result.RootRun == nil ||
		result.RootRun.State != execution.Running ||
		result.RootRun.ActiveSegmentID != "seg_root_continuation" {
		t.Fatalf("Cancel result root = %+v, want opened continuation", result.RootRun)
	}
	if prepared.committed != 1 ||
		prepared.disposition != WaitingSubtreeContinues ||
		prepared.aborted != 0 {
		t.Fatalf(
			"prepared settlement = commits:%d disposition:%d aborts:%d",
			prepared.committed,
			prepared.disposition,
			prepared.aborted,
		)
	}
	if len(effects.waitingCancels) != 1 {
		t.Fatalf("durable waiting commits = %d, want 1", len(effects.waitingCancels))
	}
	commit := effects.waitingCancels[0]
	if commit.RemainingPending != nil || commit.Resume == nil {
		t.Fatalf("continuation commit = %+v, want a tree Resume", commit)
	}
	if _, live := coordinator.registry.Get(plan.root.run.ID); !live {
		t.Fatal("continued root has no live segment owner")
	}
}

func TestCancelWaitingChildTerminalizesCommittedTreeWhenActivationFails(t *testing.T) {
	plan := waitingCancellationPlan(t, "run_a", true)
	continueErr := errors.New("continue failed")
	prepared := &fakePreparedWaitingCancellation{
		canceled:            []string{"process_a", "process_grandchild"},
		commitErr:           continueErr,
		settleOnCommitError: true,
	}
	rootAfterCommit := plan.root.run
	rootAfterCommit.State = execution.Running
	rootAfterCommit.ActiveSegmentID = "seg_root_continuation"
	rootAfterCommit.Interrupts = nil
	effects := &fakeEffects{
		waitingResult: WaitingSubtreeCancellationResult{
			TargetRun: canceledWaitingRun(
				plan.target.run,
				"stop final waiting branch",
				time.Date(2026, 7, 30, 2, 3, 4, 0, time.UTC),
			),
			RootRun: rootAfterCommit,
		},
	}
	coordinator, _ := waitingCancellationCoordinator(
		t,
		plan,
		prepared,
		effects,
		&fakeExecutor{block: true},
	)
	segmentIDs := []string{"seg_root_continuation", "seg_b_continuation"}
	coordinator.newSegmentID = func() string {
		if len(segmentIDs) == 0 {
			t.Fatal("unexpected extra segment identity")
		}
		next := segmentIDs[0]
		segmentIDs = segmentIDs[1:]
		return next
	}

	result, err := coordinator.Cancel(t.Context(), CancelCommand{
		RunID:         plan.target.run.ID,
		Reason:        "stop final waiting branch",
		AllowChildRun: true,
	})
	if err != nil {
		t.Fatalf("Cancel final waiting child: %v", err)
	}
	if result.RootRun == nil ||
		result.RootRun.ID != plan.root.run.ID ||
		result.RootRun.State != execution.Running ||
		result.RootRun.ActiveSegmentID != "seg_root_continuation" {
		t.Fatalf("Cancel result root = %+v, want exact committed continuation", result.RootRun)
	}

	coordinator.BeginShutdown()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := coordinator.AwaitShutdown(shutdownCtx); err != nil {
		t.Fatalf("await failed continuation cleanup: %v", err)
	}
	if prepared.committed != 1 ||
		prepared.disposition != WaitingSubtreeContinues ||
		prepared.aborted != 0 {
		t.Fatalf(
			"prepared settlement = commits:%d disposition:%d aborts:%d, want 1/%d/0",
			prepared.committed,
			prepared.disposition,
			prepared.aborted,
			WaitingSubtreeContinues,
		)
	}
	if len(effects.waitingCancels) != 1 {
		t.Fatalf("durable waiting commits = %d, want 1", len(effects.waitingCancels))
	}
	for _, runID := range []string{"run_b", plan.root.run.ID} {
		if !effects.terminalized(plan.pending.SessionID, runID) {
			t.Fatalf("committed continuation failure did not terminalize Run %q", runID)
		}
	}
	for _, commit := range effects.commitSnapshot() {
		if commit.State != StateTerminalize || commit.Run == nil {
			continue
		}
		if commit.Run.ID != "run_b" && commit.Run.ID != plan.root.run.ID {
			continue
		}
		if commit.Run.Outcome == nil ||
			*commit.Run.Outcome != execution.OutcomeError ||
			commit.Run.Error == nil ||
			commit.Run.Error.Kind != transcript.InternalProblem {
			t.Fatalf("failed continuation terminal = %+v, want internal error outcome", commit.Run)
		}
	}
	if _, live := coordinator.registry.Get(plan.root.run.ID); live {
		t.Fatal("failed continuation retained a live root owner")
	}
	if hasActiveSession(coordinator, plan.pending.SessionID) {
		t.Fatal("failed continuation leaked admission")
	}
}

func TestCancelWaitingChildAbortsPreparedOperationWhenDurableCommitFails(t *testing.T) {
	plan := waitingCancellationPlan(t, "run_a", false)
	prepared := &fakePreparedWaitingCancellation{
		canceled: []string{"process_a", "process_grandchild"},
		suspensions: []ProcessSuspension{{
			ProcessID:    "process_b",
			SuspensionID: "suspension_b",
			Interrupt:    waitingQuestionPrompt(),
		}},
	}
	commitErr := errors.New("durable transaction failed")
	effects := &fakeEffects{waitingErr: commitErr}
	coordinator, _ := waitingCancellationCoordinator(t, plan, prepared, effects, &fakeExecutor{})

	_, err := coordinator.Cancel(t.Context(), CancelCommand{
		RunID:         plan.target.run.ID,
		Reason:        "stop delegated branch",
		AllowChildRun: true,
	})
	if !errors.Is(err, commitErr) {
		t.Fatalf("Cancel error = %v, want durable commit failure", err)
	}
	if prepared.committed != 0 || prepared.aborted != 1 {
		t.Fatalf(
			"prepared settlement = commits:%d aborts:%d, want 0/1",
			prepared.committed,
			prepared.aborted,
		)
	}
	if hasActiveSession(coordinator, plan.pending.SessionID) {
		t.Fatal("failed waiting cancellation leaked admission")
	}
}

func waitingCancellationCoordinator(
	t *testing.T,
	plan cancellationPlan,
	prepared *fakePreparedWaitingCancellation,
	effects Effects,
	executor SegmentExecutor,
) (*Coordinator, *fakeTurnControl) {
	t.Helper()
	runsByID := make(map[string]transcript.Run)
	for _, member := range append(slices.Clone(plan.targetSubtree), plan.survivingTree...) {
		runsByID[member.run.ID] = member.run
	}
	turns := &fakeTurnControl{prepared: plan.turn}
	turns.prepareWaiting = func(
		ref execution.TurnRef,
		processID string,
	) (PreparedWaitingSubtreeCancellation, error) {
		if ref != plan.turn {
			return PreparedWaitingSubtreeCancellation{}, errors.New("prepared the wrong turn")
		}
		if processID != plan.target.processID {
			return PreparedWaitingSubtreeCancellation{}, errors.New("prepared the wrong process subtree")
		}
		return prepared.value(), nil
	}
	sessions := &fakeRunSessions{
		sess: session.Session{ID: plan.pending.SessionID, Cwd: "/work"},
		pending: map[string]interrupts.Pending{
			plan.pending.RootRunID: plan.pending,
		},
	}
	coordinator := NewCoordinator(Dependencies{
		Segments:   executor,
		Turns:      turns,
		Sessions:   sessions,
		Effects:    effects,
		Runs:       &fakeRunProjection{runs: runsByID},
		Items:      &fakeItemProjection{items: waitingCancellationItems(plan)},
		Admissions: new(admission.Gate),
		Now: func() time.Time {
			return time.Date(2026, 7, 30, 2, 3, 4, 0, time.UTC)
		},
		NewRunID:     func() string { return "run_unused" },
		NewSegmentID: func() string { return "seg_continuation" },
	})
	return coordinator, turns
}

func waitingCancellationPlan(
	t *testing.T,
	targetRunID string,
	finalBoundary bool,
) cancellationPlan {
	t.Helper()
	createdAt := time.Date(2026, 7, 30, 1, 2, 3, 0, time.UTC)
	pending := resumedTreePending(createdAt)
	if finalBoundary {
		pending.Interrupts = slices.Clone(pending.Interrupts[:1])
		pending.Suspensions = slices.Clone(pending.Suspensions[:1])
	}

	runsByID := make(map[string]transcript.Run, len(pending.Continuations))
	processes := make(map[string]string, len(pending.Continuations))
	for index := range pending.Continuations {
		continuation := &pending.Continuations[index]
		runsByID[continuation.RunID] = transcript.Run{
			ID:              continuation.RunID,
			SessionID:       pending.SessionID,
			SpawnedByItemID: continuation.Lineage.SpawnedByItemID,
			ParentRunID:     continuation.Lineage.ParentRunID,
			RootRunID:       continuation.Lineage.RootRunID,
			State:           execution.Interrupted,
			CreatedAt:       continuation.RunCreatedAt,
			UpdatedAt:       pending.CreatedAt,
			ModelSelection:  continuation.ModelSelection,
			ProtocolProfile: pending.ProtocolProfile,
			MessageMark:     transcript.UnknownMessageMark,
		}
		processes[continuation.RunID] = continuation.ProcessID
	}
	target := runsByID[targetRunID]
	if !target.Lineage().IsChild() {
		t.Fatalf("target Run %q is not a child", targetRunID)
	}
	callID := "call_" + targetRunID
	for index := range pending.Continuations {
		if pending.Continuations[index].RunID != target.ParentRunID {
			continue
		}
		pending.Continuations[index].DrainedTools = append(
			pending.Continuations[index].DrainedTools,
			interrupts.DrainedTool{
				ItemID:    target.SpawnedByItemID,
				CallID:    callID,
				Name:      "task",
				Arguments: "{}",
			},
		)
	}
	if err := pending.Validate(); err != nil {
		t.Fatalf("waiting fixture Pending: %v", err)
	}

	runValues := make([]transcript.Run, 0, len(runsByID))
	for _, run := range runsByID {
		runValues = append(runValues, run)
	}
	plan, err := newCancellationPlan(
		targetRunID,
		runValues,
		execution.TurnRef{SessionID: pending.SessionID, TurnID: pending.TurnID},
		processes,
		&pending,
	)
	if err != nil {
		t.Fatalf("waiting cancellation plan: %v", err)
	}
	plan.spawningItem = transcript.Item{
		ID:        target.SpawnedByItemID,
		SessionID: pending.SessionID,
		RunID:     target.ParentRunID,
		Status:    transcript.ItemIncomplete,
		Kind:      transcript.ToolCall,
		CreatedAt: createdAt,
		Tool: &transcript.ToolInvocation{
			Name:      "task",
			Arguments: tool.Arguments{},
		},
	}
	plan.hasSpawningItem = true
	targetRunIDs := make(map[string]struct{}, len(plan.targetSubtree))
	for _, member := range plan.targetSubtree {
		targetRunIDs[member.run.ID] = struct{}{}
	}
	for _, interrupt := range pending.Interrupts {
		if _, targeted := targetRunIDs[interrupt.RunID]; !targeted {
			continue
		}
		item := transcript.Item{
			ID:        interrupt.ItemID,
			SessionID: pending.SessionID,
			RunID:     interrupt.RunID,
			Status:    transcript.ItemRunning,
			CreatedAt: createdAt,
		}
		switch interrupt.Kind {
		case execution.QuestionInterrupt:
			item.Kind = transcript.QuestionItem
			item.Question = interrupt.Question
		case execution.ApprovalInterrupt:
			item.Kind = transcript.ToolCall
			item.Tool = &interrupt.Approval.Tool
		default:
			t.Fatalf("unsupported fixture interrupt kind %s", interrupt.Kind)
		}
		plan.targetInterruptItems = append(plan.targetInterruptItems, item)
	}
	return plan
}

func waitingCancellationItems(plan cancellationPlan) map[string]transcript.Item {
	items := make(map[string]transcript.Item, len(plan.targetInterruptItems)+1)
	items[plan.spawningItem.ID] = plan.spawningItem
	for _, item := range plan.targetInterruptItems {
		items[item.ID] = item
	}
	return items
}

func assertSettledParentTool(
	t *testing.T,
	transformation waitingCancellationTransformation,
	itemID string,
	callID string,
) {
	t.Helper()
	replacement := transformation.parentItem.Replacement
	if replacement.ID != itemID ||
		replacement.Status != transcript.ItemIncomplete ||
		replacement.Error == nil ||
		replacement.Error.Kind != transcript.ChildRunCanceledProblem ||
		replacement.Error.Scope != transcript.ToolProblem {
		t.Fatalf("parent Item replacement = %+v, want child_run_canceled", replacement)
	}
	root, ok := transformation.continuation.root()
	if !ok {
		t.Fatal("continuation has no root")
	}
	if slices.ContainsFunc(root.DrainedTools, func(tool interrupts.DrainedTool) bool {
		return tool.ItemID == itemID
	}) {
		t.Fatalf("settled Item %q remained in drained tools", itemID)
	}
	if len(root.CommittedTools) != 1 ||
		root.CommittedTools[0].ItemID != itemID ||
		root.CommittedTools[0].CallID != callID {
		t.Fatalf("committed tools = %+v, want settled parent call", root.CommittedTools)
	}
}

func runIDs(runs []transcript.Run) []string {
	ids := make([]string, len(runs))
	for index, run := range runs {
		ids[index] = run.ID
	}
	return ids
}

func continuationRunIDs(continuations []interrupts.Continuation) []string {
	ids := make([]string, len(continuations))
	for index, continuation := range continuations {
		ids[index] = continuation.RunID
	}
	return ids
}

func waitingQuestionPrompt() Interrupt {
	return Interrupt{
		Kind: execution.QuestionInterrupt,
		Question: &QuestionPrompt{
			ToolName:  "ask_user",
			Arguments: "{}",
			Fields:    []QuestionFieldSpec{{Prompt: "Continue?", Header: "Continue"}},
		},
	}
}
