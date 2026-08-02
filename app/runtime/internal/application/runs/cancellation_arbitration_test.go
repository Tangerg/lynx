package runs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

type blockingWaitingCancellationEffects struct {
	*fakeEffects
	started chan<- struct{}
	release <-chan struct{}
}

func (effects *blockingWaitingCancellationEffects) CommitWaitingSubtreeCancellation(
	ctx context.Context,
	commit WaitingSubtreeCancellationCommit,
) (WaitingSubtreeCancellationResult, error) {
	effects.started <- struct{}{}
	select {
	case <-effects.release:
	case <-ctx.Done():
		return WaitingSubtreeCancellationResult{}, ctx.Err()
	}
	return effects.fakeEffects.CommitWaitingSubtreeCancellation(ctx, commit)
}

type blockingRootCancellationSessions struct {
	*fakeRunSessions
	started chan<- struct{}
	release <-chan struct{}
	applied int
}

func (sessions *blockingRootCancellationSessions) ApplyRunCancel(
	ctx context.Context,
	sessionID string,
	runID string,
	reason string,
	finishedAt time.Time,
) (transcript.Run, error) {
	sessions.started <- struct{}{}
	select {
	case <-sessions.release:
	case <-ctx.Done():
		return transcript.Run{}, ctx.Err()
	}
	sessions.applied++
	return sessions.fakeRunSessions.ApplyRunCancel(
		ctx,
		sessionID,
		runID,
		reason,
		finishedAt,
	)
}

type cancelAttemptOutcome struct {
	result CancelResult
	err    error
}

type resumeAttemptOutcome struct {
	result StartResult
	err    error
}

func TestWaitingChildAndRootCancellationHaveOneApplicationOwner(t *testing.T) {
	t.Run("child owns the tree", func(t *testing.T) {
		plan := waitingCancellationPlan(t, "run_a", false)
		prepared := waitingCancellationMutationWithSiblingBoundary()
		baseEffects := waitingCancellationEffects(plan, "stop child")
		started := make(chan struct{}, 1)
		release := testReleaseBarrier(t)
		effects := &blockingWaitingCancellationEffects{
			fakeEffects: baseEffects,
			started:     started,
			release:     release,
		}
		coordinator, _ := waitingCancellationCoordinator(
			t,
			plan,
			prepared,
			effects,
			&fakeExecutor{},
		)

		childDone := make(chan cancelAttemptOutcome, 1)
		go func() {
			result, err := coordinator.Cancel(t.Context(), CancelCommand{
				RunID:         plan.target.run.ID,
				Reason:        "stop child",
				AllowChildRun: true,
			})
			childDone <- cancelAttemptOutcome{result: result, err: err}
		}()
		awaitTestBoundary(t, started, "child cancellation durable transaction")

		for _, command := range []CancelCommand{
			{RunID: plan.target.run.ID, Reason: "duplicate child", AllowChildRun: true},
			{RunID: plan.root.run.ID, Reason: "stop root"},
		} {
			if _, err := coordinator.Cancel(t.Context(), command); !errors.Is(err, ErrSessionBusy) {
				t.Fatalf("losing Cancel(%q) error = %v, want ErrSessionBusy", command.RunID, err)
			}
		}
		if prepared.committed != 0 {
			t.Fatalf("prepared application operation committed before durable release: %d", prepared.committed)
		}

		closeTestBarrier(release)
		outcome := awaitCancelAttempt(t, childDone)
		if outcome.err != nil {
			t.Fatalf("winning child cancellation: %v", outcome.err)
		}
		if outcome.result.Run.ID != plan.target.run.ID {
			t.Fatalf("winning child result = %+v, want %q", outcome.result, plan.target.run.ID)
		}
		if len(baseEffects.waitingCancels) != 1 || prepared.committed != 1 {
			t.Fatalf(
				"winning child commits = durable:%d runtime:%d, want 1/1",
				len(baseEffects.waitingCancels),
				prepared.committed,
			)
		}
	})

	t.Run("root owns the tree", func(t *testing.T) {
		plan := waitingCancellationPlan(t, "run_a", false)
		prepared := waitingCancellationMutationWithSiblingBoundary()
		effects := waitingCancellationEffects(plan, "stop child")
		coordinator, _ := waitingCancellationCoordinator(
			t,
			plan,
			prepared,
			effects,
			&fakeExecutor{},
		)
		baseSessions, ok := coordinator.sessions.(*fakeRunSessions)
		if !ok {
			t.Fatalf("sessions = %T, want *fakeRunSessions", coordinator.sessions)
		}
		started := make(chan struct{}, 1)
		release := testReleaseBarrier(t)
		sessions := &blockingRootCancellationSessions{
			fakeRunSessions: baseSessions,
			started:         started,
			release:         release,
		}
		coordinator.sessions = sessions

		rootDone := make(chan cancelAttemptOutcome, 1)
		go func() {
			result, err := coordinator.Cancel(t.Context(), CancelCommand{
				RunID:  plan.root.run.ID,
				Reason: "stop root",
			})
			rootDone <- cancelAttemptOutcome{result: result, err: err}
		}()
		awaitTestBoundary(t, started, "root cancellation durable transaction")

		for _, command := range []CancelCommand{
			{RunID: plan.root.run.ID, Reason: "duplicate root"},
			{RunID: plan.target.run.ID, Reason: "stop child", AllowChildRun: true},
		} {
			if _, err := coordinator.Cancel(t.Context(), command); !errors.Is(err, ErrSessionBusy) {
				t.Fatalf("losing Cancel(%q) error = %v, want ErrSessionBusy", command.RunID, err)
			}
		}
		if prepared.committed != 0 || len(effects.waitingCancels) != 0 {
			t.Fatalf(
				"losing child mutated state: durable=%d runtime=%d",
				len(effects.waitingCancels),
				prepared.committed,
			)
		}

		closeTestBarrier(release)
		outcome := awaitCancelAttempt(t, rootDone)
		if outcome.err != nil {
			t.Fatalf("winning root cancellation: %v", outcome.err)
		}
		if outcome.result.Run.ID != plan.root.run.ID || sessions.applied != 1 {
			t.Fatalf(
				"winning root result = %+v, durable commits = %d",
				outcome.result,
				sessions.applied,
			)
		}
	})
}

func TestWaitingChildCancellationAndResumeHaveOneApplicationOwner(t *testing.T) {
	t.Run("child cancellation owns the tree", func(t *testing.T) {
		plan := waitingCancellationPlan(t, "run_a", false)
		prepared := waitingCancellationMutationWithSiblingBoundary()
		baseEffects := waitingCancellationEffects(plan, "stop child")
		started := make(chan struct{}, 1)
		release := testReleaseBarrier(t)
		effects := &blockingWaitingCancellationEffects{
			fakeEffects: baseEffects,
			started:     started,
			release:     release,
		}
		coordinator, _ := waitingCancellationCoordinator(
			t,
			plan,
			prepared,
			effects,
			&fakeExecutor{},
		)

		childDone := make(chan cancelAttemptOutcome, 1)
		go func() {
			result, err := coordinator.Cancel(t.Context(), CancelCommand{
				RunID:         plan.target.run.ID,
				Reason:        "stop child",
				AllowChildRun: true,
			})
			childDone <- cancelAttemptOutcome{result: result, err: err}
		}()
		awaitTestBoundary(t, started, "child cancellation durable transaction")

		if _, err := coordinator.Resume(t.Context(), ResumeCommand{
			RunID:              plan.root.run.ID,
			CallerCapabilities: plan.pending.ProtocolProfile,
			Responses:          waitingQuestionResponses(plan.pending),
		}); !errors.Is(err, ErrSessionBusy) {
			t.Fatalf("losing Resume error = %v, want ErrSessionBusy", err)
		}

		closeTestBarrier(release)
		if outcome := awaitCancelAttempt(t, childDone); outcome.err != nil {
			t.Fatalf("winning child cancellation: %v", outcome.err)
		}
		if len(baseEffects.openings) != 0 || len(baseEffects.waitingCancels) != 1 {
			t.Fatalf(
				"commits = openings:%d waiting-cancels:%d, want 0/1",
				len(baseEffects.openings),
				len(baseEffects.waitingCancels),
			)
		}
	})

	t.Run("resume owns the tree", func(t *testing.T) {
		plan := waitingCancellationPlan(t, "run_a", false)
		prepared := waitingCancellationMutationWithSiblingBoundary()
		baseEffects := waitingCancellationEffects(plan, "stop child")
		started := make(chan struct{}, 1)
		release := testReleaseBarrier(t)
		effects := &blockingOpeningEffects{
			fakeEffects: baseEffects,
			started:     started,
			release:     release,
		}
		coordinator, turns := waitingCancellationCoordinator(
			t,
			plan,
			prepared,
			effects,
			&fakeExecutor{},
		)
		segmentIDs := []string{
			"segment_root_resumed",
			"segment_grandchild_resumed",
			"segment_a_resumed",
			"segment_b_resumed",
		}
		coordinator.newSegmentID = func() string {
			if len(segmentIDs) == 0 {
				return "unexpected_extra_segment"
			}
			next := segmentIDs[0]
			segmentIDs = segmentIDs[1:]
			return next
		}

		resumeDone := make(chan resumeAttemptOutcome, 1)
		go func() {
			result, err := coordinator.Resume(t.Context(), ResumeCommand{
				RunID:              plan.root.run.ID,
				CallerCapabilities: plan.pending.ProtocolProfile,
				Responses:          waitingQuestionResponses(plan.pending),
			})
			resumeDone <- resumeAttemptOutcome{result: result, err: err}
		}()
		awaitTestBoundary(t, started, "resume durable opening")

		if _, err := coordinator.Cancel(t.Context(), CancelCommand{
			RunID:         plan.target.run.ID,
			Reason:        "stop child",
			AllowChildRun: true,
		}); !errors.Is(err, ErrSessionBusy) {
			t.Fatalf("losing child Cancel error = %v, want ErrSessionBusy", err)
		}
		if prepared.committed != 0 || len(baseEffects.waitingCancels) != 0 {
			t.Fatalf(
				"losing child mutated state: durable=%d runtime=%d",
				len(baseEffects.waitingCancels),
				prepared.committed,
			)
		}

		closeTestBarrier(release)
		outcome := awaitResumeAttempt(t, resumeDone)
		if outcome.err != nil {
			t.Fatalf("winning Resume: %v", outcome.err)
		}
		for range outcome.result.Events {
		}
		if !turns.resumed || len(baseEffects.openings) != 1 {
			t.Fatalf(
				"winning Resume = activated:%t openings:%d, want true/1",
				turns.resumed,
				len(baseEffects.openings),
			)
		}
		if len(segmentIDs) != 0 {
			t.Fatalf("unused segment identities = %v", segmentIDs)
		}
	})
}

func TestLiveChildCancellationAndNaturalTerminalHaveOneTreeOwner(t *testing.T) {
	plan := runningChildCancellationPlan()
	completed := plan.target.run
	completed.State = execution.Completed
	outcome := execution.OutcomeCompleted
	completed.Outcome = &outcome

	t.Run("child cancellation commits first", func(t *testing.T) {
		handle := &handle{done: make(chan struct{})}
		attempt, err := handle.beginChildCancellation(plan, "stop child")
		if err != nil {
			t.Fatalf("begin child cancellation: %v", err)
		}
		canceled := plan.target.run
		canceled.State = execution.Canceled
		canceledOutcome := execution.OutcomeCanceled
		canceled.Outcome = &canceledOutcome
		handle.recordTerminalRun(canceled)
		handle.recordChildCancellationItem(
			plan.target.run.ParentRunID,
			transcript.Item{
				ID:     plan.target.run.SpawnedByItemID,
				Status: transcript.ItemIncomplete,
				Error: &transcript.Problem{
					Kind:  transcript.ChildRunCanceledProblem,
					Scope: transcript.ToolProblem,
				},
			},
		)

		target, root, err := handle.waitChildCancellation(t.Context(), attempt)
		if err != nil {
			t.Fatalf("wait child cancellation: %v", err)
		}
		if target.ID != plan.target.run.ID ||
			target.State != execution.Canceled ||
			root.ID != plan.root.run.ID {
			t.Fatalf("child cancellation result = target:%+v root:%+v", target, root)
		}
	})

	t.Run("natural terminal commits after child claim", func(t *testing.T) {
		handle := &handle{done: make(chan struct{})}
		attempt, err := handle.beginChildCancellation(plan, "stop child")
		if err != nil {
			t.Fatalf("begin child cancellation: %v", err)
		}
		if _, err := handle.beginChildCancellation(plan, "duplicate child"); !errors.Is(err, ErrSessionBusy) {
			t.Fatalf("duplicate child cancellation error = %v, want ErrSessionBusy", err)
		} else if !strings.Contains(err.Error(), plan.target.run.ID) {
			t.Fatalf("duplicate child cancellation error = %q, want target identity", err)
		}

		handle.recordTerminalRun(completed)
		if _, _, err := handle.waitChildCancellation(t.Context(), attempt); !errors.Is(err, ErrRunFinished) {
			t.Fatalf("child cancellation result = %v, want ErrRunFinished", err)
		}
		if handle.childCancel != nil {
			t.Fatal("natural terminal left a child cancellation claim")
		}
	})

	t.Run("natural terminal precedes child claim", func(t *testing.T) {
		handle := &handle{}
		handle.recordTerminalRun(completed)
		if _, err := handle.beginChildCancellation(plan, "stop child"); !errors.Is(err, ErrRunFinished) {
			t.Fatalf("child cancellation error = %v, want ErrRunFinished", err)
		}
		if handle.childCancel != nil {
			t.Fatal("finished target admitted a child cancellation claim")
		}
	})
}

func runningChildCancellationPlan() cancellationPlan {
	child := transcript.Run{
		ID:              "run_child",
		SessionID:       "session",
		SpawnedByItemID: "item_spawn",
		ParentRunID:     "run_root",
		RootRunID:       "run_root",
		State:           execution.Running,
	}
	return cancellationPlan{
		root: cancellationRun{run: transcript.Run{
			ID:        "run_root",
			SessionID: "session",
			State:     execution.Running,
		}},
		target: cancellationRun{
			run:        child,
			processID:  "process_child",
			hasProcess: true,
		},
		targetSubtree: []cancellationRun{{run: child}},
		treeState:     execution.Running,
	}
}

func waitingCancellationMutationWithSiblingBoundary() *fakePreparedWaitingCancellation {
	return &fakePreparedWaitingCancellation{
		canceled: []string{"process_a", "process_grandchild"},
		suspensions: []ProcessSuspension{{
			ProcessID:    "process_b",
			SuspensionID: "suspension_b",
			Interrupt:    waitingQuestionPrompt(),
		}},
	}
}

func waitingCancellationEffects(plan cancellationPlan, reason string) *fakeEffects {
	return &fakeEffects{
		waitingResult: WaitingSubtreeCancellationResult{
			TargetRun: canceledWaitingRun(
				plan.target.run,
				reason,
				time.Date(2026, 7, 30, 2, 3, 4, 0, time.UTC),
			),
			RootRun: plan.root.run,
		},
	}
}

func waitingQuestionResponses(pending interrupts.Pending) []ResumeResponse {
	responses := make([]ResumeResponse, len(pending.Interrupts))
	for index, interrupt := range pending.Interrupts {
		responses[index] = ResumeResponse{
			ItemID:   interrupt.ItemID,
			Kind:     QuestionResponseKind,
			Question: &QuestionResponse{Answers: [][]string{{"continue"}}},
		}
	}
	return responses
}

func testReleaseBarrier(t *testing.T) chan struct{} {
	t.Helper()
	release := make(chan struct{})
	t.Cleanup(func() {
		closeTestBarrier(release)
	})
	return release
}

func closeTestBarrier(release chan struct{}) {
	select {
	case <-release:
	default:
		close(release)
	}
}

func awaitTestBoundary(t *testing.T, started <-chan struct{}, name string) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-started:
	case <-timer.C:
		t.Fatalf("%s was not reached", name)
	}
}

func awaitCancelAttempt(
	t *testing.T,
	done <-chan cancelAttemptOutcome,
) cancelAttemptOutcome {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case outcome := <-done:
		return outcome
	case <-timer.C:
		t.Fatal("Cancel did not finish")
		return cancelAttemptOutcome{}
	}
}

func awaitResumeAttempt(
	t *testing.T,
	done <-chan resumeAttemptOutcome,
) resumeAttemptOutcome {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case outcome := <-done:
		return outcome
	case <-timer.C:
		t.Fatal("Resume did not finish")
		return resumeAttemptOutcome{}
	}
}
