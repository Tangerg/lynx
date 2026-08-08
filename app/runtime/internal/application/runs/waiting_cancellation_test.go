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
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
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
	canceled      []string
	interruptions []MemberInterruption
	checkpoint    *ExecutorCheckpoint
	commitErr     error
	// settleOnCommitError models the executor's post-commit Continue
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
		CanceledMemberIDs:    slices.Clone(prepared.canceled),
		PendingInterruptions: slices.Clone(prepared.interruptions),
		Checkpoint:           checkpoint,
		Mutation:             prepared,
	}
}

func TestPrepareWaitingCancellationRejectsCheckpointBoundToDifferentApplicationFacts(t *testing.T) {
	plan := waitingCancellationPlan(t, "run_a", false)
	for name, mutate := range map[string]func(*ExecutorCheckpoint){
		"root":       func(checkpoint *ExecutorCheckpoint) { checkpoint.RootMemberID = "other_root" },
		"session":    func(checkpoint *ExecutorCheckpoint) { checkpoint.Scope.SessionID = "other_session" },
		"goal lease": func(checkpoint *ExecutorCheckpoint) { checkpoint.Scope.GoalLeaseID = "other_goal" },
		"provider": func(checkpoint *ExecutorCheckpoint) {
			checkpoint.ModelSelection = mustSelection("anthropic", checkpoint.ModelSelection.Model())
		},
		"model": func(checkpoint *ExecutorCheckpoint) {
			checkpoint.ModelSelection = mustSelection(checkpoint.ModelSelection.Provider(), "other-model")
		},
	} {
		t.Run(name, func(t *testing.T) {
			checkpoint := testExecutorCheckpoint()
			mutate(&checkpoint)
			prepared := &fakePreparedWaitingCancellation{
				canceled:   []string{"member_a", "member_grandchild"},
				checkpoint: &checkpoint,
			}
			_, err := prepareWaitingCancellationTransformation(
				plan,
				"stop delegated branch",
				time.Date(2026, 7, 30, 2, 3, 4, 0, time.UTC),
				prepared.value(),
			)
			if !errors.Is(err, ErrInvalidExecutorCheckpoint) {
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
		canceled: []string{"member_a", "member_grandchild"},
		interruptions: []MemberInterruption{{
			MemberID:  "member_b",
			RequestID: "request_b",
			Interrupt: waitingQuestionPrompt(),
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
	for _, record := range transformation.terminalRuns {
		if record.State != run.Canceled ||
			record.Outcome == nil ||
			*record.Outcome != run.OutcomeCanceled ||
			record.Detail != "stop delegated branch" ||
			!record.FinishedAt.Equal(finishedAt) {
			t.Fatalf("terminal Run = %+v, want exact canceled snapshot", record)
		}
	}
}

func TestPrepareWaitingCancellationContinuesAfterFinalBoundaryIsRemoved(t *testing.T) {
	plan := waitingCancellationPlan(t, "run_a", true)
	prepared := &fakePreparedWaitingCancellation{
		canceled: []string{"member_a", "member_grandchild"},
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
		interruptions  []MemberInterruption
		affectedRunIDs []string
	}{
		{
			name: "remaining waiting boundary",
			interruptions: []MemberInterruption{{
				MemberID:  "member_b",
				RequestID: "request_b",
				Interrupt: waitingQuestionPrompt(),
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
				canceled:      []string{"member_a", "member_grandchild"},
				interruptions: test.interruptions,
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
		canceled: []string{"member_a", "member_grandchild"},
		interruptions: []MemberInterruption{{
			MemberID:  "member_b",
			RequestID: "request_b",
			Interrupt: waitingQuestionPrompt(),
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
	coordinator, control := waitingCancellationCoordinator(t, plan, prepared, effects, &fakeExecutor{})

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
		result.RootRun.State != run.Waiting {
		t.Fatalf("Cancel result = %+v, want canceled child and waiting root", result)
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
	if control.prepared != plan.executor {
		t.Fatalf("prepared execution = %+v, want %+v", control.prepared, plan.executor)
	}
}

func TestCancelWaitingChildRecoversCommittedTreeWhenRuntimeApplyFails(t *testing.T) {
	plan := waitingCancellationPlan(t, "run_a", false)
	applyErr := errors.New("runtime apply failed")
	prepared := &fakePreparedWaitingCancellation{
		canceled:  []string{"member_a", "member_grandchild"},
		commitErr: applyErr,
		interruptions: []MemberInterruption{{
			MemberID:  "member_b",
			RequestID: "request_b",
			Interrupt: waitingQuestionPrompt(),
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
	coordinator, control := waitingCancellationCoordinator(t, plan, prepared, effects, &fakeExecutor{})
	sessions := coordinator.sessionReader.(*fakeRunSessions)
	var operations []string
	sessions.operations = &operations
	control.operations = &operations

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
	if !reflect.DeepEqual(control.released, []ExecutorRef{plan.executor}) {
		t.Fatalf("released execution = %+v, want [%+v]", control.released, plan.executor)
	}
	if !slices.Equal(operations, []string{"durable.lost", "executor.release"}) {
		t.Fatalf("recovery operations = %v, want durable.lost then executor.release", operations)
	}
}

func TestCancelWaitingChildRestoresParkedExecutorAfterRuntimeRestart(t *testing.T) {
	plan := waitingCancellationPlan(t, "run_a", false)
	prepared := &fakePreparedWaitingCancellation{
		canceled: []string{"member_a", "member_grandchild"},
		interruptions: []MemberInterruption{{
			MemberID:  "member_b",
			RequestID: "request_b",
			Interrupt: waitingQuestionPrompt(),
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
	coordinator, control := waitingCancellationCoordinator(t, plan, prepared, effects, &fakeExecutor{})
	control.prepared = ExecutorRef{}
	control.prepareErr = ErrExecutorNotLive
	control.rehydrated = plan.executor

	if _, err := coordinator.Cancel(t.Context(), CancelCommand{
		RunID:         plan.target.run.ID,
		Reason:        "stop after restart",
		AllowChildRun: true,
	}); err != nil {
		t.Fatalf("Cancel rehydrated waiting child: %v", err)
	}
	rootContinuation, _ := plan.pending.RootContinuation()
	if control.rehydrateReq.SessionID != plan.pending.SessionID ||
		control.rehydrateReq.ExecutorID != plan.pending.ExecutorID ||
		control.rehydrateReq.MemberID != rootContinuation.MemberID ||
		control.rehydrateReq.CWD != "/work" ||
		control.rehydrateReq.ModelSelection != rootContinuation.ModelSelection {
		t.Fatalf("rehydrate request = %+v, want durable root continuation", control.rehydrateReq)
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
		canceled: []string{"member_a", "member_grandchild"},
	}
	rootAfterCommit := plan.root.run
	rootAfterCommit.State = run.Running
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
		result.RootRun.State != run.Running ||
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
		canceled:            []string{"member_a", "member_grandchild"},
		commitErr:           continueErr,
		settleOnCommitError: true,
	}
	rootAfterCommit := plan.root.run
	rootAfterCommit.State = run.Running
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
		result.RootRun.State != run.Running ||
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
			*commit.Run.Outcome != run.OutcomeFailed ||
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
		canceled: []string{"member_a", "member_grandchild"},
		interruptions: []MemberInterruption{{
			MemberID:  "member_b",
			RequestID: "request_b",
			Interrupt: waitingQuestionPrompt(),
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
	effects completeTestProjectionPorts,
	executor interface {
		ExecutionObserver
		ExecutionReleaser
	},
) (*Coordinator, *fakeExecutionPorts) {
	t.Helper()
	runsByID := make(map[string]transcript.Run)
	for _, member := range append(slices.Clone(plan.targetSubtree), plan.survivingTree...) {
		runsByID[member.run.ID] = member.run
	}
	control := &fakeExecutionPorts{prepared: plan.executor}
	control.prepareWaiting = func(
		ref ExecutorRef,
		memberID string,
	) (PreparedWaitingSubtreeCancellation, error) {
		if ref != plan.executor {
			return PreparedWaitingSubtreeCancellation{}, errors.New("prepared the wrong execution")
		}
		if memberID != plan.target.memberID {
			return PreparedWaitingSubtreeCancellation{}, errors.New("prepared the wrong process subtree")
		}
		return prepared.value(), nil
	}
	sessions := &fakeRunSessions{
		sess: session.Session{ID: plan.pending.SessionID, CWD: "/work"},
		pending: map[string]Pending{
			plan.pending.RootRunID: plan.pending,
		},
	}
	coordinator := NewCoordinator(Dependencies{
		Observations:  executor,
		Releases:      control,
		Continuation:  control,
		LegacyWaiting: control,
		RunningTrees:  control,
		WaitingTrees:  control,
		Session:       testSessionPorts(sessions),
		Projection:    testProjectionPorts(effects),
		Runs:          &fakeRunProjection{runs: runsByID},
		Items:         &fakeItemProjection{items: waitingCancellationItems(plan)},
		Admissions:    new(admission.Gate),
		Now: func() time.Time {
			return time.Date(2026, 7, 30, 2, 3, 4, 0, time.UTC)
		},
		NewRunID:     func() string { return "run_unused" },
		NewSegmentID: func() string { return "seg_continuation" },
	})
	return coordinator, control
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
		pending.Bindings = slices.Clone(pending.Bindings[:1])
	}

	runsByID := make(map[string]transcript.Run, len(pending.Continuations))
	members := make(map[string]string, len(pending.Continuations))
	for index := range pending.Continuations {
		continuation := &pending.Continuations[index]
		runsByID[continuation.RunID] = transcript.Run{
			ID:              continuation.RunID,
			SessionID:       pending.SessionID,
			SpawnedByItemID: continuation.Lineage.SpawnedByItemID,
			ParentRunID:     continuation.Lineage.ParentRunID,
			RootRunID:       continuation.Lineage.RootRunID,
			State:           run.Waiting,
			CreatedAt:       continuation.RunCreatedAt,
			UpdatedAt:       pending.CreatedAt,
			ModelSelection:  continuation.ModelSelection,
			Capabilities:    pending.Capabilities,
			MessageMark:     transcript.UnknownMessageMark,
		}
		members[continuation.RunID] = continuation.MemberID
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
			DrainedTool{
				ItemID: target.SpawnedByItemID, ItemOccurredAt: createdAt,
				CallID: callID, Name: "delegate_task", Arguments: "{}",
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
		ExecutorRef{SessionID: pending.SessionID, ExecutorID: pending.ExecutorID},
		members,
		&pending,
	)
	if err != nil {
		t.Fatalf("waiting cancellation plan: %v", err)
	}
	plan.spawningItem = transcript.Item{
		ID:         target.SpawnedByItemID,
		SessionID:  pending.SessionID,
		RunID:      target.ParentRunID,
		Status:     transcript.ItemIncomplete,
		Kind:       transcript.ToolCall,
		OccurredAt: createdAt,
		FinishedAt: createdAt,
		Tool: &transcript.ToolInvocation{
			Name:      "delegate_task",
			Arguments: tool.Arguments{},
		},
	}
	plan.hasSpawningItem = true
	targetRunIDs := make(map[string]struct{}, len(plan.targetSubtree))
	for _, member := range plan.targetSubtree {
		targetRunIDs[member.run.ID] = struct{}{}
	}
	for _, pendingInterrupt := range pending.Interrupts {
		if _, targeted := targetRunIDs[pendingInterrupt.RunID]; !targeted {
			continue
		}
		item := transcript.Item{
			ID:         pendingInterrupt.ItemID,
			SessionID:  pending.SessionID,
			RunID:      pendingInterrupt.RunID,
			Status:     transcript.ItemRunning,
			OccurredAt: createdAt,
		}
		switch pendingInterrupt.Kind {
		case interrupt.Question:
			item.Kind = transcript.QuestionItem
			item.Question = pendingInterrupt.Question
		case interrupt.Approval:
			item.Kind = transcript.ToolCall
			item.Tool = &pendingInterrupt.Approval.Tool
		default:
			t.Fatalf("unsupported fixture interrupt kind %s", pendingInterrupt.Kind)
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
	if slices.ContainsFunc(root.DrainedTools, func(tool DrainedTool) bool {
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

func continuationRunIDs(continuations []Continuation) []string {
	ids := make([]string, len(continuations))
	for index, continuation := range continuations {
		ids[index] = continuation.RunID
	}
	return ids
}

func waitingQuestionPrompt() Interrupt {
	return Interrupt{
		Kind: interrupt.Question,
		Question: &QuestionPrompt{
			ToolName:  "ask_user",
			Arguments: "{}",
			Fields:    []QuestionFieldSpec{{Prompt: "Continue?", Header: "Continue"}},
		},
	}
}
