package runs

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

type recoveryStoreStub struct {
	runs         []transcript.Run
	pending      []interrupts.Pending
	transcripts  map[string][]transcript.Item
	messageMarks map[string]int
	sessions     map[string]session.Session

	commit    RecoveryCommit
	commits   int
	commitErr error
}

func (store *recoveryStoreStub) ListNonTerminalRuns(context.Context) ([]transcript.Run, error) {
	return append([]transcript.Run(nil), store.runs...), nil
}

func (store *recoveryStoreStub) ListPendingInterrupts(context.Context) ([]interrupts.Pending, error) {
	return append([]interrupts.Pending(nil), store.pending...), nil
}

func (store *recoveryStoreStub) GetSession(_ context.Context, sessionID string) (session.Session, error) {
	if sess, ok := store.sessions[sessionID]; ok {
		return sess, nil
	}
	return session.Session{ID: sessionID, Cwd: "/workspace"}, nil
}

func (store *recoveryStoreStub) ListTranscript(_ context.Context, sessionID string) ([]transcript.Item, error) {
	return append([]transcript.Item(nil), store.transcripts[sessionID]...), nil
}

func (store *recoveryStoreStub) CountMessages(_ context.Context, sessionID string) (int, error) {
	return store.messageMarks[sessionID], nil
}

func (store *recoveryStoreStub) CommitRecovery(_ context.Context, commit RecoveryCommit) error {
	store.commits++
	store.commit = commit
	return store.commitErr
}

type checkpointResumabilityFunc func(context.Context, execution.ExecutorCheckpointExpectation) (bool, error)

func (validate checkpointResumabilityFunc) CanResumeCheckpoint(
	ctx context.Context,
	expected execution.ExecutorCheckpointExpectation,
) (bool, error) {
	return validate(ctx, expected)
}

func TestRecoveryMarksAbandonedRunTreeLostInPostorder(t *testing.T) {
	createdAt := time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC)
	finishedAt := createdAt.Add(time.Minute)
	root := transcript.Run{
		ID: "run_root", SessionID: "session", State: execution.Running,
		ActiveSegmentID: "segment_root", CreatedAt: createdAt, MessageMark: transcript.UnknownMessageMark,
	}
	child := transcript.Run{
		ID: "run_child", SessionID: root.SessionID, State: execution.Running,
		ActiveSegmentID: "segment_child", ParentRunID: root.ID, RootRunID: root.ID,
		SpawnedByItemID: "item_spawn", CreatedAt: createdAt, MessageMark: transcript.UnknownMessageMark,
	}
	item := transcript.Item{
		ID: "item_running", SessionID: root.SessionID, RunID: child.ID,
		Kind: transcript.QuestionItem, Status: transcript.ItemRunning, CreatedAt: createdAt,
		Question: &transcript.Question{Prompt: "Continue?"},
	}
	store := &recoveryStoreStub{
		runs:         []transcript.Run{root, child},
		transcripts:  map[string][]transcript.Item{root.SessionID: {item}},
		messageMarks: map[string]int{root.SessionID: 7},
	}
	checkpointCalls := 0
	recovery, err := NewRecovery(store, checkpointResumabilityFunc(func(context.Context, execution.ExecutorCheckpointExpectation) (bool, error) {
		checkpointCalls++
		return true, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}
	recovery.now = func() time.Time { return finishedAt }

	recovered, err := recovery.Reconcile(t.Context())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if recovered != 2 || store.commits != 1 || checkpointCalls != 0 {
		t.Fatalf("recovered/commits/checkpointCalls = %d/%d/%d, want 2/1/0", recovered, store.commits, checkpointCalls)
	}
	if got := []string{store.commit.LostRuns[0].ID, store.commit.LostRuns[1].ID}; !reflect.DeepEqual(got, []string{child.ID, root.ID}) {
		t.Fatalf("lost Run order = %v, want child-before-parent", got)
	}
	for _, lost := range store.commit.LostRuns {
		if lost.State != execution.Failed ||
			lost.Outcome == nil || *lost.Outcome != execution.OutcomeError ||
			lost.Error == nil || lost.Error.Kind != transcript.RunLostProblem ||
			lost.MessageMark != 7 || !lost.FinishedAt.Equal(finishedAt) {
			t.Fatalf("lost Run = %+v", lost)
		}
	}
	if len(store.commit.ItemReplacements) != 1 ||
		!reflect.DeepEqual(store.commit.ItemReplacements[0].Expected, item) ||
		store.commit.ItemReplacements[0].Replacement.Status != transcript.ItemIncomplete {
		t.Fatalf("Item replacements = %+v", store.commit.ItemReplacements)
	}
	if len(store.commit.PreservedCheckpointRootIDs) != 0 {
		t.Fatalf("preserved checkpoints = %v, want none", store.commit.PreservedCheckpointRootIDs)
	}
}

func TestRecoveryChargesLostGoalOwnedRootToItsAdmissionLease(t *testing.T) {
	createdAt := time.Date(2026, 8, 1, 1, 30, 0, 0, time.UTC)
	finishedAt := createdAt.Add(time.Minute)
	cost := 1.25
	run := transcript.Run{
		ID: "run_goal", SessionID: "session", State: execution.Running,
		ActiveSegmentID: "segment_goal", GoalLeaseID: "lease_goal",
		Metrics: transcript.RunMetrics{
			Steps: 3,
			Usage: &transcript.Usage{ModelUsage: transcript.ModelUsage{CostUSD: &cost}},
		},
		CreatedAt: createdAt, MessageMark: transcript.UnknownMessageMark,
	}
	store := &recoveryStoreStub{
		runs:         []transcript.Run{run},
		transcripts:  map[string][]transcript.Item{run.SessionID: nil},
		messageMarks: map[string]int{run.SessionID: 2},
	}
	recovery, err := NewRecovery(store, checkpointResumabilityFunc(func(context.Context, execution.ExecutorCheckpointExpectation) (bool, error) {
		return false, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}
	recovery.now = func() time.Time { return finishedAt }

	if _, err := recovery.Reconcile(t.Context()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(store.commit.GoalTurns) != 1 {
		t.Fatalf("goal turns = %+v, want one", store.commit.GoalTurns)
	}
	turn := store.commit.GoalTurns[0]
	if turn.SessionID != run.SessionID || turn.LeaseID != run.GoalLeaseID ||
		turn.RunID != run.ID || turn.Outcome != execution.OutcomeError ||
		turn.CostUSD != cost || turn.Steps != run.Metrics.Steps ||
		!turn.CompletedAt.Equal(finishedAt) {
		t.Fatalf("goal turn = %+v", turn)
	}

	missingCharge := store.commit
	missingCharge.GoalTurns = nil
	if err := missingCharge.Validate(); err == nil {
		t.Fatal("RecoveryCommit.Validate accepted a lost goal-owned Run without its charge")
	}
	mismatchedCharge := store.commit
	mismatchedCharge.GoalTurns = append([]goal.TurnRecord(nil), store.commit.GoalTurns...)
	mismatchedCharge.GoalTurns[0].LeaseID = "other-lease"
	if err := mismatchedCharge.Validate(); err == nil {
		t.Fatal("RecoveryCommit.Validate accepted a Goal turn from another lease")
	}
	foreignDeletion := store.commit
	foreignDeletion.DeletePending = append(
		[]PendingDeletion(nil),
		store.commit.DeletePending...,
	)
	foreignDeletion.DeletePending = append(foreignDeletion.DeletePending, PendingDeletion{
		SessionID: "other-session", RootRunID: "run_foreign",
	})
	if err := foreignDeletion.Validate(); err == nil {
		t.Fatal("RecoveryCommit.Validate accepted deletion of an unrelated Pending set")
	}
}

func TestRecoveryPreservesOnlyCoherentInterruptedTree(t *testing.T) {
	run, pending, item := coherentRecoveryPark(t)
	pending.GoalLeaseID = "goal-lease-1"
	run.GoalLeaseID = pending.GoalLeaseID
	store := &recoveryStoreStub{
		runs:         []transcript.Run{run},
		pending:      []interrupts.Pending{pending},
		transcripts:  map[string][]transcript.Item{run.SessionID: {item}},
		messageMarks: map[string]int{run.SessionID: 3},
	}
	var validated execution.ExecutorCheckpointExpectation
	recovery, err := NewRecovery(store, checkpointResumabilityFunc(func(_ context.Context, expected execution.ExecutorCheckpointExpectation) (bool, error) {
		validated = expected
		return true, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	recovered, err := recovery.Reconcile(t.Context())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	wantExpectation := execution.ExecutorCheckpointExpectation{
		RootProcessID:  "process_root",
		SessionID:      run.SessionID,
		Cwd:            "/workspace",
		GoalLeaseID:    pending.GoalLeaseID,
		ModelSelection: run.ModelSelection,
		Limits:         run.Limits,
	}
	if recovered != 0 || validated != wantExpectation || len(store.commit.LostRuns) != 0 {
		t.Fatalf("recovery = %d validated=%+v commit=%+v", recovered, validated, store.commit)
	}
	if !reflect.DeepEqual(store.commit.PreservedCheckpointRootIDs, []string{"process_root"}) ||
		len(store.commit.DeletePending) != 0 {
		t.Fatalf("ownership plan = %+v", store.commit)
	}
}

func TestRecoveryMarksIsolatedParkLostWithoutProbingExecutorCheckpoint(t *testing.T) {
	run, pending, item := coherentRecoveryPark(t)
	store := &recoveryStoreStub{
		runs:        []transcript.Run{run},
		pending:     []interrupts.Pending{pending},
		transcripts: map[string][]transcript.Item{run.SessionID: {item}},
		sessions: map[string]session.Session{
			run.SessionID: {ID: run.SessionID, Cwd: "/workspace", Isolated: true},
		},
	}
	checkpointCalls := 0
	recovery, err := NewRecovery(store, checkpointResumabilityFunc(func(context.Context, execution.ExecutorCheckpointExpectation) (bool, error) {
		checkpointCalls++
		return true, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	recovered, err := recovery.Reconcile(t.Context())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if recovered != 1 || checkpointCalls != 0 || len(store.commit.LostRuns) != 1 {
		t.Fatalf("recovered=%d checkpointCalls=%d commit=%+v", recovered, checkpointCalls, store.commit)
	}
}

func TestRecoveryTreatsUnavailableExecutorCheckpointAsResourceLoss(t *testing.T) {
	run, pending, item := coherentRecoveryPark(t)
	store := &recoveryStoreStub{
		runs:         []transcript.Run{run},
		pending:      []interrupts.Pending{pending},
		transcripts:  map[string][]transcript.Item{run.SessionID: {item}},
		messageMarks: map[string]int{run.SessionID: 5},
	}
	recovery, err := NewRecovery(store, checkpointResumabilityFunc(func(context.Context, execution.ExecutorCheckpointExpectation) (bool, error) {
		return false, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	recovered, err := recovery.Reconcile(t.Context())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if recovered != 1 || len(store.commit.LostRuns) != 1 ||
		!reflect.DeepEqual(store.commit.DeletePending, []PendingDeletion{{SessionID: run.SessionID, RootRunID: run.ID}}) ||
		len(store.commit.PreservedCheckpointRootIDs) != 0 {
		t.Fatalf("resource-loss recovery = %d, commit %+v", recovered, store.commit)
	}
}

func TestRecoveryValidationFailureDoesNotCommitPartialRepair(t *testing.T) {
	run, pending, item := coherentRecoveryPark(t)
	store := &recoveryStoreStub{
		runs:        []transcript.Run{run},
		pending:     []interrupts.Pending{pending},
		transcripts: map[string][]transcript.Item{run.SessionID: {item}},
	}
	want := errors.New("checkpoint backend unavailable")
	recovery, err := NewRecovery(store, checkpointResumabilityFunc(func(context.Context, execution.ExecutorCheckpointExpectation) (bool, error) {
		return false, want
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	if _, err := recovery.Reconcile(t.Context()); !errors.Is(err, want) {
		t.Fatalf("Reconcile error = %v, want %v", err, want)
	}
	if store.commits != 0 {
		t.Fatalf("CommitRecovery calls = %d, want none", store.commits)
	}
}

func TestRecoveryRejectsCrossSessionPendingWithoutCommit(t *testing.T) {
	run, pending, item := coherentRecoveryPark(t)
	pending.SessionID = "other-session"
	store := &recoveryStoreStub{
		runs:        []transcript.Run{run},
		pending:     []interrupts.Pending{pending},
		transcripts: map[string][]transcript.Item{run.SessionID: {item}},
	}
	checkpointCalls := 0
	recovery, err := NewRecovery(store, checkpointResumabilityFunc(func(context.Context, execution.ExecutorCheckpointExpectation) (bool, error) {
		checkpointCalls++
		return true, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	if _, err := recovery.Reconcile(t.Context()); err == nil {
		t.Fatal("Reconcile accepted a Pending owned by another Session")
	}
	if store.commits != 0 || checkpointCalls != 0 {
		t.Fatalf("recovery mutated or probed executor after corruption: commits=%d checkpointCalls=%d", store.commits, checkpointCalls)
	}
}

// TestRecoveryRejectsContinuationFactDriftWithoutProbingCheckpoint proves
// parked_continuation_matches_run_facts at boot recovery: contradictory facts
// fail before an executor probe or durable repair can turn them into history.
func TestRecoveryRejectsContinuationFactDriftWithoutProbingCheckpoint(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*transcript.Run, *interrupts.Pending)
	}{
		{
			name: "cumulative metrics",
			mutate: func(_ *transcript.Run, pending *interrupts.Pending) {
				pending.Continuations[0].Metrics.Steps++
			},
		},
		{
			name: "frozen limits",
			mutate: func(_ *transcript.Run, pending *interrupts.Pending) {
				pending.Continuations[0].Limits.MaxSteps++
			},
		},
		{
			name: "frozen protocol profile",
			mutate: func(run *transcript.Run, _ *interrupts.Pending) {
				run.ProtocolProfile.ChildRuns = true
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run, pending, item := coherentRecoveryPark(t)
			test.mutate(&run, &pending)
			store := &recoveryStoreStub{
				runs:        []transcript.Run{run},
				pending:     []interrupts.Pending{pending},
				transcripts: map[string][]transcript.Item{run.SessionID: {item}},
			}
			checkpointCalls := 0
			recovery, err := NewRecovery(store, checkpointResumabilityFunc(func(context.Context, execution.ExecutorCheckpointExpectation) (bool, error) {
				checkpointCalls++
				return true, nil
			}))
			if err != nil {
				t.Fatalf("NewRecovery: %v", err)
			}

			if _, err := recovery.Reconcile(t.Context()); err == nil {
				t.Fatal("Reconcile accepted a continuation fact that differs from Run admission")
			}
			if store.commits != 0 || checkpointCalls != 0 {
				t.Fatalf("recovery mutated or probed after fact drift: commits=%d checkpointCalls=%d", store.commits, checkpointCalls)
			}
		})
	}
}

// TestRecoveryRejectsChildProtocolDriftWithoutProbingCheckpoint proves
// parked_continuation_matches_run_facts for root-owned policy: every child Run
// is parked under the root admission, even though Continuation does not repeat
// that policy as a second source of truth.
func TestRecoveryRejectsChildProtocolDriftWithoutProbingCheckpoint(t *testing.T) {
	root, pending, item := coherentRecoveryPark(t)
	root.ProtocolProfile.ChildRuns = true
	pending.ProtocolProfile.ChildRuns = true
	lineage := execution.RunLineage{
		SpawnedByItemID: "item_spawn",
		ParentRunID:     root.ID,
		RootRunID:       root.ID,
	}
	child := transcript.Run{
		ID: "run_child", SessionID: root.SessionID, State: execution.Interrupted,
		SpawnedByItemID: lineage.SpawnedByItemID,
		ParentRunID:     lineage.ParentRunID,
		RootRunID:       lineage.RootRunID,
		ModelSelection:  root.ModelSelection,
		// This is a valid profile in isolation but contradicts the root admission.
		ProtocolProfile: execution.RunProtocolProfile{
			InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
		},
		CreatedAt: root.CreatedAt, MessageMark: transcript.UnknownMessageMark,
	}
	rootContinuation := pending.Continuations[0]
	pending.Continuations = []interrupts.Continuation{
		{
			RunID: "run_child", ProcessID: "process_child",
			Lineage: lineage, ModelSelection: root.ModelSelection,
			RunCreatedAt: root.CreatedAt,
		},
		rootContinuation,
	}
	store := &recoveryStoreStub{
		runs:        []transcript.Run{root, child},
		pending:     []interrupts.Pending{pending},
		transcripts: map[string][]transcript.Item{root.SessionID: {item}},
	}
	checkpointCalls := 0
	recovery, err := NewRecovery(store, checkpointResumabilityFunc(func(context.Context, execution.ExecutorCheckpointExpectation) (bool, error) {
		checkpointCalls++
		return true, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	if _, err := recovery.Reconcile(t.Context()); err == nil {
		t.Fatal("Reconcile accepted a child Run protocol profile that differs from root admission")
	}
	if store.commits != 0 || checkpointCalls != 0 {
		t.Fatalf("recovery mutated or probed after child policy drift: commits=%d checkpointCalls=%d", store.commits, checkpointCalls)
	}
}

func coherentRecoveryPark(t *testing.T) (transcript.Run, interrupts.Pending, transcript.Item) {
	t.Helper()
	createdAt := time.Date(2026, 8, 1, 2, 0, 0, 0, time.UTC)
	selection, err := modelref.New("anthropic", "claude")
	if err != nil {
		t.Fatalf("model selection: %v", err)
	}
	question := &transcript.Question{Prompt: "Continue?"}
	interrupt := transcript.Interrupt{
		ItemID: "item_question", RunID: "run_root", Kind: execution.QuestionInterrupt, Question: question,
	}
	run := transcript.Run{
		ID: "run_root", SessionID: "session", State: execution.Interrupted,
		ModelSelection: selection, Interrupts: []transcript.Interrupt{interrupt},
		ProtocolProfile: execution.RunProtocolProfile{InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt}},
		CreatedAt:       createdAt, UpdatedAt: createdAt.Add(time.Second), MessageMark: transcript.UnknownMessageMark,
	}
	pending := interrupts.Pending{
		RootRunID:  run.ID,
		SessionID:  run.SessionID,
		TurnID:     "turn_root",
		Interrupts: []transcript.Interrupt{interrupt},
		ProtocolProfile: execution.RunProtocolProfile{
			InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
		},
		Suspensions: []interrupts.SuspensionBinding{{
			InterruptItemID: interrupt.ItemID,
			ProcessID:       "process_root",
			SuspensionID:    "suspension_root",
		}},
		Continuations: []interrupts.Continuation{{
			RunID: run.ID, ProcessID: "process_root", ModelSelection: selection, RunCreatedAt: createdAt,
		}},
		CreatedAt: createdAt.Add(time.Second),
	}
	if err := pending.Validate(); err != nil {
		t.Fatalf("Pending fixture: %v", err)
	}
	item := transcript.Item{
		ID: interrupt.ItemID, SessionID: run.SessionID, RunID: run.ID,
		Kind: transcript.QuestionItem, Status: transcript.ItemRunning,
		Question: question, CreatedAt: pending.CreatedAt,
	}
	return run, pending, item
}
