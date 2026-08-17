package runs

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessionadmission"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/itemfixture"
	runfixture "github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
	corechat "github.com/Tangerg/lynx/core/chat"
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
	applyErr      error
	continueErr   error

	disposition WaitingSubtreeDisposition
	applied     int
	continued   int
	discarded   int
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
		Change:               prepared,
	}
}

func TestPrepareWaitingCancellationRejectsCheckpointBoundToDifferentApplicationFacts(t *testing.T) {
	plan := runACancellationPlan(t, false)
	for name, mutate := range map[string]func(*ExecutorCheckpoint){
		"root":             func(checkpoint *ExecutorCheckpoint) { checkpoint.RootMemberID = "other_root" },
		"session":          func(checkpoint *ExecutorCheckpoint) { checkpoint.Scope.SessionID = "other_session" },
		"goal incarnation": func(checkpoint *ExecutorCheckpoint) { checkpoint.Scope.GoalIncarnationID = "other_goal" },
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
			if prepared.applied != 0 || prepared.discarded != 0 {
				t.Fatalf("mutation touched before ownership validation: applied=%d discarded=%d", prepared.applied, prepared.discarded)
			}
		})
	}
}

func (prepared *fakePreparedWaitingCancellation) Apply(
	disposition WaitingSubtreeDisposition,
) error {
	prepared.applied++
	prepared.disposition = disposition
	if prepared.applyErr != nil {
		return prepared.applyErr
	}
	prepared.settled = true
	return nil
}

func (prepared *fakePreparedWaitingCancellation) Continue(context.Context) error {
	if !prepared.settled || prepared.disposition != WaitingSubtreeResumesRunning {
		return errors.New("fake waiting subtree cancellation was not applied for continuation")
	}
	prepared.continued++
	return prepared.continueErr
}

func (prepared *fakePreparedWaitingCancellation) Discard() error {
	if prepared.settled {
		return nil
	}
	prepared.discarded++
	prepared.settled = true
	return nil
}

func TestPrepareWaitingCancellationKeepsSurvivingExternalBoundary(t *testing.T) {
	plan := runACancellationPlan(t, false)
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
	if len(transformation.terminalItems) != 1 ||
		transformation.terminalItems[0].Expected.ID() != "item_target_tool" ||
		transformation.terminalItems[0].Replacement.Status() != transcript.ItemIncomplete {
		t.Fatalf("terminal Tool Items = %+v, want canceled branch Tool settled once", transformation.terminalItems)
	}
	if len(transformation.conversationMessages) != 1 ||
		transformation.conversationMessages[0].Role != corechat.RoleTool ||
		transformation.conversationMessages[0].Parts[0].ToolResult == nil ||
		transformation.conversationMessages[0].Parts[0].ToolResult.ID != "provider_run_a" {
		t.Fatalf("conversation Messages = %+v, want parent delegate Tool result", transformation.conversationMessages)
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
		if record.State() != run.Canceled ||
			!runHasOutcome(record, run.OutcomeCanceled) ||
			record.Detail() != "stop delegated branch" ||
			!record.FinishedAt().Equal(finishedAt) {
			t.Fatalf("terminal Run = %+v, want exact canceled snapshot", record)
		}
	}
}

func TestPrepareWaitingCancellationContinuesAfterFinalBoundaryIsRemoved(t *testing.T) {
	plan := runACancellationPlan(t, true)
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
			plan := runACancellationPlan(t, test.finalBoundary)
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
			invalidations := &invalidationRecorder{}
			coordinator := &Coordinator{invalidations: invalidations.publish}

			coordinator.publishWaitingChildCancellation(plan, transformation)

			want := []invalidation.Notice{
				invalidation.InSession(invalidation.Runs, plan.pending.SessionID, test.affectedRunIDs...),
				invalidation.InSession(invalidation.Interrupts, plan.pending.SessionID, plan.root.run.ID()),
				invalidation.InSession(invalidation.Sessions, plan.pending.SessionID),
			}
			if got := invalidations.snapshot(); !reflect.DeepEqual(got, want) {
				t.Fatalf("published notices = %+v, want %+v", got, want)
			}
		})
	}
}

func TestCancelWaitingChildCommitsReducedPendingBeforeRuntimeTransition(t *testing.T) {
	plan := runACancellationPlan(t, false)
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
			TargetRun: mustCanceledWaitingRun(t,
				plan.target.run,
				"stop delegated branch",
				time.Date(2026, 7, 30, 2, 3, 4, 0, time.UTC),
			),
			RootRun: plan.root.run,
		},
	}
	coordinator, control := waitingCancellationCoordinator(t, plan, prepared, effects, &fakeExecutor{})

	result, err := coordinator.Cancel(t.Context(), CancelCommand{
		RunID:         plan.target.run.ID(),
		Reason:        "stop delegated branch",
		AllowChildRun: true,
	})
	if err != nil {
		t.Fatalf("Cancel waiting child: %v", err)
	}
	if result.Run.ID() != plan.target.run.ID() ||
		result.RootRun == nil ||
		result.RootRun.State() != run.Waiting {
		t.Fatalf("Cancel result = %+v, want canceled child and waiting root", result)
	}
	if prepared.applied != 1 ||
		prepared.disposition != WaitingSubtreeStaysWaiting ||
		prepared.discarded != 0 {
		t.Fatalf(
			"prepared settlement = applies:%d disposition:%d discards:%d",
			prepared.applied,
			prepared.disposition,
			prepared.discarded,
		)
	}
	if len(effects.waitingCancels) != 1 || effects.waitingCancels[0].CommitID == "" ||
		effects.waitingCancels[0].RemainingPending == nil {
		t.Fatalf("durable waiting commits = %+v, want one reduced Pending", effects.waitingCancels)
	}
	if control.prepared != plan.executor {
		t.Fatalf("prepared execution = %+v, want %+v", control.prepared, plan.executor)
	}
}

func TestCancelWaitingChildRestoresCommittedTreeWhenRuntimeApplyFails(t *testing.T) {
	plan := runACancellationPlan(t, false)
	applyErr := errors.New("runtime apply failed")
	prepared := &fakePreparedWaitingCancellation{
		canceled: []string{"member_a", "member_grandchild"},
		applyErr: applyErr,
		interruptions: []MemberInterruption{{
			MemberID:  "member_b",
			RequestID: "request_b",
			Interrupt: waitingQuestionPrompt(),
		}},
	}
	effects := &fakeEffects{
		waitingResult: WaitingSubtreeCancellationResult{
			TargetRun: mustCanceledWaitingRun(t,
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

	result, err := coordinator.Cancel(t.Context(), CancelCommand{
		RunID:         plan.target.run.ID(),
		Reason:        "stop delegated branch",
		AllowChildRun: true,
	})
	if err != nil {
		t.Fatalf("Cancel after exact waiting-tree recovery: %v", err)
	}
	if prepared.applied != 1 || prepared.discarded != 1 {
		t.Fatalf(
			"prepared settlement = applies:%d discards:%d, want 1/1",
			prepared.applied,
			prepared.discarded,
		)
	}
	if sessions.lostRunID != "" {
		t.Fatalf("recovered waiting tree was incorrectly marked lost as %q", sessions.lostRunID)
	}
	if !reflect.DeepEqual(control.released, []ExecutorRef{plan.executor}) {
		t.Fatalf("released execution = %+v, want [%+v]", control.released, plan.executor)
	}
	if len(control.restoreWaiting) != 1 ||
		control.restoreWaiting[0].Checkpoint.RootMemberID != plan.pending.Continuations[len(plan.pending.Continuations)-1].MemberID {
		t.Fatalf("restored waiting continuation = %+v, want committed resulting checkpoint", control.restoreWaiting)
	}
	if result.Run.ID() != plan.target.run.ID() || result.RootRun == nil || result.RootRun.ID() != plan.root.run.ID() {
		t.Fatalf("Cancel result = %+v, want committed child/root result", result)
	}
	if !slices.Equal(operations, []string{"executor.release", "executor.restore_waiting"}) {
		t.Fatalf("recovery operations = %v, want release then exact restore", operations)
	}
}

func TestCancelWaitingChildMarksRunLostOnlyWhenCommittedCheckpointCannotRestore(t *testing.T) {
	plan := runACancellationPlan(t, false)
	applyErr := errors.New("runtime apply failed")
	restoreErr := errors.New("committed checkpoint is corrupt")
	prepared := &fakePreparedWaitingCancellation{
		canceled: []string{"member_a", "member_grandchild"},
		applyErr: applyErr,
		interruptions: []MemberInterruption{{
			MemberID: "member_b", RequestID: "request_b", Interrupt: waitingQuestionPrompt(),
		}},
	}
	effects := &fakeEffects{waitingResult: WaitingSubtreeCancellationResult{
		TargetRun: mustCanceledWaitingRun(t,
			plan.target.run,
			"stop delegated branch",
			time.Date(2026, 7, 30, 2, 3, 4, 0, time.UTC),
		),
		RootRun: plan.root.run,
	}}
	coordinator, control := waitingCancellationCoordinator(t, plan, prepared, effects, &fakeExecutor{})
	sessions := coordinator.sessionReader.(*fakeRunSessions)
	var operations []string
	sessions.operations = &operations
	control.operations = &operations
	control.restoreErr = restoreErr

	_, err := coordinator.Cancel(t.Context(), CancelCommand{
		RunID: plan.target.run.ID(), Reason: "stop delegated branch", AllowChildRun: true,
	})
	if !errors.Is(err, applyErr) || !errors.Is(err, restoreErr) {
		t.Fatalf("Cancel error = %v, want apply and restore failures", err)
	}
	if sessions.lostRunID != plan.root.run.ID() {
		t.Fatalf("lost Run = %q, want root %q", sessions.lostRunID, plan.root.run.ID())
	}
	if !slices.Equal(operations, []string{
		"executor.release", "executor.restore_waiting", "durable.lost",
	}) {
		t.Fatalf("recovery operations = %v, want release, restore, then durable lost", operations)
	}
}

func TestCancelWaitingChildPassesDurableTreeToExecutorAfterRuntimeRestart(t *testing.T) {
	plan := runACancellationPlan(t, false)
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
			TargetRun: mustCanceledWaitingRun(t,
				plan.target.run,
				"stop after restart",
				time.Date(2026, 7, 30, 2, 3, 4, 0, time.UTC),
			),
			RootRun: plan.root.run,
		},
	}
	coordinator, control := waitingCancellationCoordinator(t, plan, prepared, effects, &fakeExecutor{})
	originalPrepare := control.prepareWaiting
	var request WaitingSubtreeCancellationRequest
	control.prepareWaiting = func(
		candidate WaitingSubtreeCancellationRequest,
	) (PreparedWaitingSubtreeCancellation, error) {
		request = candidate
		return originalPrepare(candidate)
	}

	if _, err := coordinator.Cancel(t.Context(), CancelCommand{
		RunID:         plan.target.run.ID(),
		Reason:        "stop after restart",
		AllowChildRun: true,
	}); err != nil {
		t.Fatalf("Cancel restored waiting child: %v", err)
	}
	rootContinuation, _ := plan.pending.RootContinuation()
	continuation := request.Continuation
	if continuation.SessionID != plan.pending.SessionID ||
		continuation.ExecutorID != plan.pending.ExecutorID ||
		continuation.Checkpoint.RootMemberID != rootContinuation.MemberID ||
		continuation.Checkpoint.Scope.CWD != "/work" ||
		continuation.Checkpoint.ModelSelection != rootContinuation.ModelSelection {
		t.Fatalf("waiting subtree request = %+v, want durable root continuation", request)
	}
	if prepared.applied != 1 ||
		prepared.disposition != WaitingSubtreeStaysWaiting {
		t.Fatalf(
			"restored prepared settlement = applies:%d disposition:%d",
			prepared.applied,
			prepared.disposition,
		)
	}
}

func TestCancelWaitingChildOpensContinuationWhenFinalBoundaryIsRemoved(t *testing.T) {
	plan := runACancellationPlan(t, true)
	prepared := &fakePreparedWaitingCancellation{
		canceled: []string{"member_a", "member_grandchild"},
	}
	rootAfterCommit := plan.root.run
	rootAfterCommit = mustResumeWaitingRun(t, rootAfterCommit, "seg_root_continuation")
	effects := &fakeEffects{
		waitingResult: WaitingSubtreeCancellationResult{
			TargetRun: mustCanceledWaitingRun(t,
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
		RunID:         plan.target.run.ID(),
		Reason:        "stop final waiting branch",
		AllowChildRun: true,
	})
	if err != nil {
		t.Fatalf("Cancel final waiting child: %v", err)
	}
	if result.RootRun == nil ||
		result.RootRun.State() != run.Running ||
		result.RootRun.ActiveSegmentID() != "seg_root_continuation" {
		t.Fatalf("Cancel result root = %+v, want opened continuation", result.RootRun)
	}
	if prepared.applied != 1 ||
		prepared.disposition != WaitingSubtreeResumesRunning ||
		prepared.continued != 1 ||
		prepared.discarded != 0 {
		t.Fatalf(
			"prepared settlement = applies:%d disposition:%d continues:%d discards:%d",
			prepared.applied,
			prepared.disposition,
			prepared.continued,
			prepared.discarded,
		)
	}
	if len(effects.waitingCancels) != 1 {
		t.Fatalf("durable waiting commits = %d, want 1", len(effects.waitingCancels))
	}
	commit := effects.waitingCancels[0]
	if commit.CommitID == "" || commit.RemainingPending != nil || commit.Resume == nil {
		t.Fatalf("continuation commit = %+v, want a tree Resume", commit)
	}
	if _, live := coordinator.registry.Get(plan.root.run.ID()); !live {
		t.Fatal("continued root has no live segment owner")
	}
}

func TestCancelWaitingChildTerminalizesCommittedTreeWhenActivationFails(t *testing.T) {
	plan := runACancellationPlan(t, true)
	continueErr := errors.New("continue failed")
	prepared := &fakePreparedWaitingCancellation{
		canceled:    []string{"member_a", "member_grandchild"},
		continueErr: continueErr,
	}
	rootAfterCommit := plan.root.run
	rootAfterCommit = mustResumeWaitingRun(t, rootAfterCommit, "seg_root_continuation")
	effects := &fakeEffects{
		waitingResult: WaitingSubtreeCancellationResult{
			TargetRun: mustCanceledWaitingRun(t,
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
		RunID:         plan.target.run.ID(),
		Reason:        "stop final waiting branch",
		AllowChildRun: true,
	})
	if err != nil {
		t.Fatalf("Cancel final waiting child: %v", err)
	}
	if result.RootRun == nil ||
		result.RootRun.ID() != plan.root.run.ID() ||
		result.RootRun.State() != run.Running ||
		result.RootRun.ActiveSegmentID() != "seg_root_continuation" {
		t.Fatalf("Cancel result root = %+v, want exact committed continuation", result.RootRun)
	}

	coordinator.BeginShutdown()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := coordinator.AwaitShutdown(shutdownCtx); err != nil {
		t.Fatalf("await failed continuation cleanup: %v", err)
	}
	if prepared.applied != 1 ||
		prepared.disposition != WaitingSubtreeResumesRunning ||
		prepared.continued != 1 ||
		prepared.discarded != 0 {
		t.Fatalf(
			"prepared settlement = applies:%d disposition:%d continues:%d discards:%d, want 1/%d/1/0",
			prepared.applied,
			prepared.disposition,
			prepared.continued,
			prepared.discarded,
			WaitingSubtreeResumesRunning,
		)
	}
	if len(effects.waitingCancels) != 1 {
		t.Fatalf("durable waiting commits = %d, want 1", len(effects.waitingCancels))
	}
	for _, runID := range []string{"run_b", plan.root.run.ID()} {
		if !effects.terminalized(plan.pending.SessionID, runID) {
			t.Fatalf("committed continuation failure did not terminalize Run %q", runID)
		}
	}
	for _, commit := range effects.commitSnapshot() {
		if commit.State != StateTerminalize || commit.Run == nil {
			continue
		}
		if commit.Run.ID() != "run_b" && commit.Run.ID() != plan.root.run.ID() {
			continue
		}
		if !runHasOutcome(*commit.Run, run.OutcomeFailed) ||
			!runHasFailureKind(*commit.Run, run.FailureInternal) {
			t.Fatalf("failed continuation terminal = %+v, want internal error outcome", commit.Run)
		}
	}
	if _, live := coordinator.registry.Get(plan.root.run.ID()); live {
		t.Fatal("failed continuation retained a live root owner")
	}
	if hasActiveSession(coordinator, plan.pending.SessionID) {
		t.Fatal("failed continuation leaked admission")
	}
}

func TestCancelWaitingChildAbortsPreparedOperationWhenDurableCommitFails(t *testing.T) {
	plan := runACancellationPlan(t, false)
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
		RunID:         plan.target.run.ID(),
		Reason:        "stop delegated branch",
		AllowChildRun: true,
	})
	if !errors.Is(err, commitErr) {
		t.Fatalf("Cancel error = %v, want durable commit failure", err)
	}
	if prepared.applied != 0 || prepared.discarded != 1 {
		t.Fatalf(
			"prepared settlement = applies:%d discards:%d, want 0/1",
			prepared.applied,
			prepared.discarded,
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
	runsByID := make(map[string]run.Run)
	for _, member := range append(slices.Clone(plan.targetSubtree), plan.survivingTree...) {
		runsByID[member.run.ID()] = member.run
	}
	control := &fakeExecutionPorts{prepared: plan.executor}
	control.prepareWaiting = func(
		request WaitingSubtreeCancellationRequest,
	) (PreparedWaitingSubtreeCancellation, error) {
		ref := ExecutorRef{
			SessionID:  request.Continuation.SessionID,
			ExecutorID: request.Continuation.ExecutorID,
		}
		if ref != plan.executor {
			return PreparedWaitingSubtreeCancellation{}, errors.New("prepared the wrong execution")
		}
		if request.TargetMemberID != plan.target.memberID {
			return PreparedWaitingSubtreeCancellation{}, errors.New("prepared the wrong executor member subtree")
		}
		if request.Reason == "" {
			return PreparedWaitingSubtreeCancellation{}, errors.New("prepared without a cancellation reason")
		}
		return prepared.value(), nil
	}
	sessions := &fakeRunSessions{
		sess: sessionfixture.MustRestore(session.Snapshot{ID: plan.pending.SessionID, CWD: "/work"}),
		pending: map[string]Pending{
			plan.pending.RootRunID: plan.pending,
		},
	}
	coordinator := mustNewCoordinator(Dependencies{
		Observations:                       executor,
		Releases:                           control,
		Continuation:                       control,
		WaitingRestorer:                    control,
		RunningSubtreeCanceler:             control,
		WaitingSubtreeCancellationPreparer: control,
		Session:                            testSessionPorts(sessions),
		Projection:                         testProjectionPorts(effects),
		Runs:                               &fakeRunProjection{runs: runsByID},
		Items:                              &fakeItemProjection{items: waitingCancellationItems(plan)},
		Admissions:                         new(sessionadmission.Gate),
		Now: func() time.Time {
			return time.Date(2026, 7, 30, 2, 3, 4, 0, time.UTC)
		},
		NewRunID:     func() string { return "run_unused" },
		NewSegmentID: func() string { return "seg_continuation" },
	})
	return coordinator, control
}

func runACancellationPlan(
	t *testing.T,
	finalBoundary bool,
) cancellationPlan {
	t.Helper()
	const targetRunID = "run_a"

	createdAt := time.Date(2026, 7, 30, 1, 2, 3, 0, time.UTC)
	pending := resumedTreePending(createdAt)
	if finalBoundary {
		pending.Interrupts = slices.Clone(pending.Interrupts[:1])
		pending.Bindings = slices.Clone(pending.Bindings[:1])
	}

	runsByID := make(map[string]run.Run, len(pending.Continuations))
	members := make(map[string]string, len(pending.Continuations))
	for index := range pending.Continuations {
		continuation := &pending.Continuations[index]
		runsByID[continuation.RunID] = runfixture.MustRestore(run.Snapshot{ID: continuation.RunID,
			SessionID: pending.SessionID,

			State:          run.Waiting,
			CreatedAt:      continuation.RunCreatedAt,
			UpdatedAt:      pending.CreatedAt,
			ModelSelection: continuation.ModelSelection,
			Capabilities:   pending.Capabilities,
			MessageMark:    run.UnknownMessageMark, Lineage: run.Lineage{SpawnedByItemID: continuation.Lineage.SpawnedByItemID,
				ParentRunID: continuation.Lineage.ParentRunID,
				RootRunID:   continuation.Lineage.RootRunID}})

		members[continuation.RunID] = continuation.MemberID
	}
	target := runsByID[targetRunID]
	if !target.Lineage().IsChild() {
		t.Fatalf("target Run %q is not a child", targetRunID)
	}
	callID := "call_" + targetRunID
	for index := range pending.Continuations {
		if pending.Continuations[index].RunID != target.Lineage().ParentRunID {
			if pending.Continuations[index].RunID == targetRunID {
				pending.Continuations[index].DrainedTools = append(
					pending.Continuations[index].DrainedTools,
					DrainedTool{
						ItemID: "item_target_tool", ItemOccurredAt: createdAt,
						CallID: "call_target_tool", SourceCallID: "provider_target_tool",
						Name: "ask_user", Arguments: "{}",
					},
				)
			}
			continue
		}
		pending.Continuations[index].DrainedTools = append(
			pending.Continuations[index].DrainedTools,
			DrainedTool{
				ItemID: target.Lineage().SpawnedByItemID, ItemOccurredAt: createdAt,
				CallID: callID, SourceCallID: "provider_" + targetRunID,
				Name: "delegate_task", Arguments: "{}",
			},
		)
	}
	if err := pending.Validate(); err != nil {
		t.Fatalf("waiting fixture Pending: %v", err)
	}

	runValues := make([]run.Run, 0, len(runsByID))
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
	plan.spawningItem = itemfixture.MustRestore(itemfixture.Input{
		ID:         target.Lineage().SpawnedByItemID,
		SessionID:  pending.SessionID,
		RunID:      target.Lineage().ParentRunID,
		Status:     transcript.ItemRunning,
		Kind:       transcript.ToolCall,
		OccurredAt: createdAt,
		Tool: &transcript.ToolInvocation{
			Name:      "delegate_task",
			Arguments: tool.Arguments{},
		},
	})
	plan.hasSpawningItem = true
	targetRunIDs := make(map[string]struct{}, len(plan.targetSubtree))
	for _, member := range plan.targetSubtree {
		targetRunIDs[member.run.ID()] = struct{}{}
	}
	for _, pendingInterrupt := range pending.Interrupts {
		if _, targeted := targetRunIDs[pendingInterrupt.RunID]; !targeted {
			continue
		}
		input := itemfixture.Input{
			ID: pendingInterrupt.ItemID, SessionID: pending.SessionID,
			RunID: pendingInterrupt.RunID, OccurredAt: createdAt,
		}
		switch pendingInterrupt.Kind {
		case interrupt.Question:
			input.Kind = transcript.QuestionItem
			input.Status = transcript.ItemCompleted
			input.Question = pendingInterrupt.Question
		case interrupt.Approval:
			input.Kind = transcript.ToolCall
			input.Status = transcript.ItemRunning
			input.Tool = &pendingInterrupt.Approval.Tool
		default:
			t.Fatalf("unsupported fixture interrupt kind %s", pendingInterrupt.Kind)
		}
		item := itemfixture.MustRestore(input)
		plan.targetInterruptItems = append(plan.targetInterruptItems, item)
	}
	for _, continuation := range pending.Continuations {
		if _, targeted := targetRunIDs[continuation.RunID]; !targeted {
			continue
		}
		for _, drained := range continuation.DrainedTools {
			plan.targetDrainedItems = append(plan.targetDrainedItems, itemfixture.MustRestore(itemfixture.Input{
				ID: drained.ItemID, SessionID: pending.SessionID, RunID: continuation.RunID,
				Status: transcript.ItemRunning, Kind: transcript.ToolCall, OccurredAt: drained.ItemOccurredAt,
				Tool: &transcript.ToolInvocation{Name: drained.Name},
			}))
		}
	}
	return plan
}

func waitingCancellationItems(plan cancellationPlan) map[string]transcript.Item {
	items := make(map[string]transcript.Item, len(plan.targetInterruptItems)+len(plan.targetDrainedItems)+1)
	items[plan.spawningItem.ID()] = plan.spawningItem
	for _, item := range plan.targetInterruptItems {
		items[item.ID()] = item
	}
	for _, item := range plan.targetDrainedItems {
		items[item.ID()] = item
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
	failure, failed := replacement.Failure()
	if replacement.ID() != itemID || replacement.Status() != transcript.ItemIncomplete ||
		!failed || failure.Kind != tool.FailureChildRunCanceled {
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

func runIDs(runs []run.Run) []string {
	ids := make([]string, len(runs))
	for index, record := range runs {
		ids[index] = record.ID()
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

func mustCanceledWaitingRun(t *testing.T, record run.Run, reason string, finishedAt time.Time) run.Run {
	t.Helper()
	canceled, err := canceledWaitingRun(record, reason, finishedAt)
	if err != nil {
		t.Fatalf("cancel waiting Run: %v", err)
	}
	return canceled
}

func mustResumeWaitingRun(t *testing.T, record run.Run, segmentID string) run.Run {
	t.Helper()
	resumed, err := record.Resume(segmentID, record.UpdatedAt())
	if err != nil {
		t.Fatalf("resume waiting Run: %v", err)
	}
	return resumed
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
