package runs

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessionadmission"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	interruptdomain "github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/itemfixture"
	runfixture "github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
	corechat "github.com/Tangerg/lynx/core/chat"
)

func newTestRecovery(
	store RecoveryStore,
	resumability WaitingExecutionResumability,
) (*Recovery, error) {
	return NewRecovery(store, resumability, new(sessionadmission.Gate), nil)
}

type recoveryStoreStub struct {
	runs         []rundomain.Run
	pending      []Pending
	models       []OpenModelInvocation
	tools        []OpenToolInvocation
	transcripts  map[string][]transcript.Item
	messageMarks map[string]int
	messages     map[string][]corechat.Message
	sessions     map[string]session.Session

	commit        RecoveryCommit
	commits       int
	commitErr     error
	checkpointErr error
}

func (store *recoveryStoreStub) ListNonTerminalRuns(context.Context) ([]rundomain.Run, error) {
	return append([]rundomain.Run(nil), store.runs...), nil
}

func (store *recoveryStoreStub) ListPendingInterrupts(context.Context) ([]Pending, error) {
	return append([]Pending(nil), store.pending...), nil
}

func (store *recoveryStoreStub) ListOpenModelInvocations(context.Context) ([]OpenModelInvocation, error) {
	return append([]OpenModelInvocation(nil), store.models...), nil
}

func (store *recoveryStoreStub) ListOpenToolInvocations(context.Context) ([]OpenToolInvocation, error) {
	return append([]OpenToolInvocation(nil), store.tools...), nil
}

func (store *recoveryStoreStub) SessionByID(_ context.Context, sessionID string) (session.Session, error) {
	if sess, ok := store.sessions[sessionID]; ok {
		return sess, nil
	}
	return sessionfixture.MustRestore(session.Snapshot{ID: sessionID, CWD: "/workspace"}), nil
}

func (store *recoveryStoreStub) ListTranscript(_ context.Context, sessionID string) ([]transcript.Item, error) {
	return append([]transcript.Item(nil), store.transcripts[sessionID]...), nil
}

func (store *recoveryStoreStub) CountMessages(_ context.Context, sessionID string) (int, error) {
	if _, explicit := store.messageMarks[sessionID]; !explicit {
		return len(store.messages[sessionID]), nil
	}
	return store.messageMarks[sessionID], nil
}

func (store *recoveryStoreStub) ReadMessages(
	_ context.Context,
	sessionID string,
) ([]corechat.Message, error) {
	if messages, explicit := store.messages[sessionID]; explicit {
		cloned := make([]corechat.Message, len(messages))
		for index, message := range messages {
			cloned[index] = message.Clone()
		}
		return cloned, nil
	}
	messages := make([]corechat.Message, store.messageMarks[sessionID])
	for index := range messages {
		messages[index] = corechat.NewUserMessage(corechat.NewTextPart(fmt.Sprintf("message %d", index+1)))
	}
	return messages, nil
}

func (store *recoveryStoreStub) LoadExecutorCheckpoint(
	_ context.Context,
	rootMemberID string,
) (ExecutorCheckpoint, error) {
	if store.checkpointErr != nil {
		return ExecutorCheckpoint{}, store.checkpointErr
	}
	for _, pending := range store.pending {
		root, found := pending.RootContinuation()
		if !found || root.MemberID != rootMemberID {
			continue
		}
		sess, found := store.sessions[pending.SessionID]
		if !found {
			sess = sessionfixture.MustRestore(session.Snapshot{ID: pending.SessionID, CWD: "/workspace"})
		}
		return ExecutorCheckpoint{
			RootMemberID: rootMemberID,
			Payload:      []byte(`{}`),
			BuildID:      "test-build",
			Scope: ExecutionScope{
				SessionID: pending.SessionID, CWD: sess.CWD(), WorkspaceCWD: sess.CWD(),
				Isolated: sess.Isolated(), GoalIncarnationID: pending.GoalIncarnationID,
			},
			ModelSelection: root.ModelSelection,
			Limits:         root.Limits,
			Capabilities:   pending.Capabilities,
		}, nil
	}
	return ExecutorCheckpoint{}, ErrExecutorCheckpointNotFound
}

func (store *recoveryStoreStub) CommitRecovery(_ context.Context, commit RecoveryCommit) error {
	store.commits++
	store.commit = commit
	return store.commitErr
}

type waitingExecutionResumabilityFunc func(context.Context, WaitingContinuation) (bool, error)

func (validate waitingExecutionResumabilityFunc) CanResumeWaitingExecution(
	ctx context.Context,
	continuation WaitingContinuation,
) (bool, error) {
	return validate(ctx, continuation)
}

type selectiveRecoveryAdmissions struct {
	busy     map[string]bool
	released map[string]int
}

func (a *selectiveRecoveryAdmissions) AcquireSession(sessionID string) (func(), bool) {
	if a.busy[sessionID] {
		return nil, false
	}
	return func() { a.released[sessionID]++ }, true
}

func TestRecoverySkipsFactsOwnedByAnotherRuntime(t *testing.T) {
	createdAt := time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC)
	recoverable := runfixture.MustRestore(rundomain.Snapshot{
		ID: "run_recoverable", SessionID: "session_recoverable", State: rundomain.Running,
		ActiveSegmentID: "segment_recoverable", CreatedAt: createdAt,
		MessageMark: rundomain.UnknownMessageMark,
	})
	foreign := runfixture.MustRestore(rundomain.Snapshot{
		ID: "run_foreign", SessionID: "session_foreign", State: rundomain.Running,
		ActiveSegmentID: "segment_foreign", CreatedAt: createdAt,
		MessageMark: rundomain.UnknownMessageMark,
	})
	store := &recoveryStoreStub{
		runs: []rundomain.Run{recoverable, foreign},
		models: []OpenModelInvocation{
			{SessionID: recoverable.SessionID(), RunID: recoverable.ID(), SegmentID: "segment_recoverable", CallID: "call_recoverable", StartedAt: createdAt},
			{SessionID: foreign.SessionID(), RunID: foreign.ID(), SegmentID: "segment_foreign", CallID: "call_foreign", StartedAt: createdAt},
		},
		transcripts:  map[string][]transcript.Item{},
		messageMarks: map[string]int{},
	}
	admissions := &selectiveRecoveryAdmissions{
		busy: map[string]bool{foreign.SessionID(): true}, released: map[string]int{},
	}
	var notices []invalidation.Notice
	recovery, err := NewRecovery(
		store,
		waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
			return true, nil
		}),
		admissions,
		func(notice invalidation.Notice) { notices = append(notices, notice) },
	)
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}
	recovery.now = func() time.Time { return createdAt.Add(time.Minute) }
	reconciled, err := recovery.Reconcile(t.Context())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if reconciled != 1 || len(store.commit.LostRuns) != 1 || store.commit.LostRuns[0].ID() != recoverable.ID() {
		t.Fatalf("recovery touched wrong Runs: reconciled=%d lost=%+v", reconciled, store.commit.LostRuns)
	}
	if len(store.commit.ModelInvocations) != 1 || store.commit.ModelInvocations[0].RunID != recoverable.ID() {
		t.Fatalf("recovery touched foreign invocation: %+v", store.commit.ModelInvocations)
	}
	if !reflect.DeepEqual(store.commit.RecoveredSessionIDs, []string{recoverable.SessionID()}) ||
		!reflect.DeepEqual(store.commit.DeleteCheckpointSessionIDs, []string{recoverable.SessionID()}) {
		t.Fatalf("recovery cleanup scope = sessions:%v checkpoints:%v", store.commit.RecoveredSessionIDs, store.commit.DeleteCheckpointSessionIDs)
	}
	if admissions.released[recoverable.SessionID()] != 1 || admissions.released[foreign.SessionID()] != 0 {
		t.Fatalf("recovery releases = %+v", admissions.released)
	}
	wantNotices := []invalidation.Notice{
		invalidation.InSession(invalidation.Runs, recoverable.SessionID(), recoverable.ID()),
		invalidation.InSession(invalidation.Interrupts, recoverable.SessionID(), recoverable.ID()),
		invalidation.InSession(invalidation.Sessions, recoverable.SessionID()),
	}
	if !reflect.DeepEqual(notices, wantNotices) {
		t.Fatalf("recovery notices = %+v, want %+v", notices, wantNotices)
	}
}

func TestRecoveryDoesNotPublishBeforeItsCommitSucceeds(t *testing.T) {
	createdAt := time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC)
	abandoned := runfixture.MustRestore(rundomain.Snapshot{
		ID: "run_abandoned", SessionID: "session_abandoned", State: rundomain.Running,
		ActiveSegmentID: "segment_abandoned", CreatedAt: createdAt,
		MessageMark: rundomain.UnknownMessageMark,
	})
	commitErr := errors.New("commit failed")
	store := &recoveryStoreStub{
		runs: []rundomain.Run{abandoned}, transcripts: map[string][]transcript.Item{},
		messageMarks: map[string]int{}, commitErr: commitErr,
	}
	var notices []invalidation.Notice
	recovery, err := NewRecovery(
		store,
		waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
			return true, nil
		}),
		new(sessionadmission.Gate),
		func(notice invalidation.Notice) { notices = append(notices, notice) },
	)
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}
	if _, err := recovery.Reconcile(t.Context()); !errors.Is(err, commitErr) {
		t.Fatalf("Reconcile error = %v, want %v", err, commitErr)
	}
	if len(notices) != 0 {
		t.Fatalf("failed recovery published notices: %+v", notices)
	}
}

func TestRecoveryMarksAbandonedRunTreeLostInPostorder(t *testing.T) {
	createdAt := time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC)
	finishedAt := createdAt.Add(time.Minute)
	root := runfixture.MustRestore(rundomain.Snapshot{ID: "run_root", SessionID: "session", State: rundomain.Running,
		ActiveSegmentID: "segment_root", CreatedAt: createdAt, MessageMark: rundomain.UnknownMessageMark})

	child := runfixture.MustRestore(rundomain.Snapshot{ID: "run_child", SessionID: root.SessionID(), State: rundomain.Running,
		ActiveSegmentID: "segment_child",
		CreatedAt:       createdAt, MessageMark: rundomain.UnknownMessageMark, Lineage: rundomain.Lineage{ParentRunID: root.ID(), RootRunID: root.ID(),
			SpawnedByItemID: "item_spawn"}})

	item := itemfixture.MustRestore(itemfixture.Input{
		ID: "item_running", SessionID: root.SessionID(), RunID: child.ID(),
		Kind: transcript.QuestionItem, OccurredAt: createdAt,
		Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}},
	})
	store := &recoveryStoreStub{
		runs: []rundomain.Run{root, child},
		models: []OpenModelInvocation{
			{SessionID: root.SessionID(), RunID: root.ID(), SegmentID: "segment_root", CallID: "model_root", StartedAt: createdAt.Add(time.Second)},
			{SessionID: root.SessionID(), RunID: "run_already_terminal", SegmentID: "segment_old", CallID: "model_orphan", StartedAt: createdAt.Add(2 * time.Second)},
		},
		tools: []OpenToolInvocation{{
			SessionID: child.SessionID(), RunID: child.ID(), SegmentID: "segment_child",
			CallID: "tool_child", ItemID: "item_tool_child", StartedAt: createdAt.Add(3 * time.Second),
		}},
		transcripts:  map[string][]transcript.Item{root.SessionID(): {item}},
		messageMarks: map[string]int{root.SessionID(): 7},
	}
	checkpointCalls := 0
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
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
	if got := []string{store.commit.LostRuns[0].ID(), store.commit.LostRuns[1].ID()}; !reflect.DeepEqual(got, []string{child.ID(), root.ID()}) {
		t.Fatalf("lost Run order = %v, want child-before-parent", got)
	}
	for _, lost := range store.commit.LostRuns {
		if lost.State() != rundomain.Failed || !runHasOutcome(lost, rundomain.OutcomeLost) ||
			!runHasFailureKind(lost, rundomain.FailureLost) ||
			lost.MessageMark() != 7 || !lost.FinishedAt().Equal(finishedAt) {
			t.Fatalf("lost Run = %+v", lost)
		}
	}
	if len(store.commit.ItemReplacements) != 0 {
		t.Fatalf("Item replacements = %+v, complete Question prompt must remain unchanged", store.commit.ItemReplacements)
	}
	if len(store.commit.PreservedCheckpointRootIDs) != 0 {
		t.Fatalf("preserved checkpoints = %v, want none", store.commit.PreservedCheckpointRootIDs)
	}
	if len(store.commit.ModelInvocations) != 2 || len(store.commit.ToolInvocations) != 1 {
		t.Fatalf(
			"recovered invocations = model:%+v Tool:%+v",
			store.commit.ModelInvocations,
			store.commit.ToolInvocations,
		)
	}
	for _, invocation := range store.commit.ModelInvocations {
		if !invocation.FinishedAt.Equal(finishedAt) {
			t.Fatalf("model invocation recovery = %+v", invocation)
		}
	}
	if !store.commit.ToolInvocations[0].FinishedAt.Equal(finishedAt) {
		t.Fatalf("Tool invocation recovery = %+v", store.commit.ToolInvocations[0])
	}
}

func TestRecoveryDoesNotMoveDurableTimeBackwardWhenTheClockRegresses(t *testing.T) {
	base := time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		updatedAt time.Time
		itemAt    time.Time
		modelAt   time.Time
		toolAt    time.Time
		want      time.Time
	}{
		{name: "Run update", updatedAt: base.Add(2 * time.Minute), want: base.Add(2 * time.Minute)},
		{name: "Transcript Item", itemAt: base.Add(3 * time.Minute), want: base.Add(3 * time.Minute)},
		{name: "model attempt", modelAt: base.Add(4 * time.Minute), want: base.Add(4 * time.Minute)},
		{name: "Tool attempt", toolAt: base.Add(5 * time.Minute), want: base.Add(5 * time.Minute)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			updatedAt := test.updatedAt
			if updatedAt.IsZero() {
				updatedAt = base
			}
			active := runfixture.MustRestore(rundomain.Snapshot{
				ID: "run", SessionID: "session", State: rundomain.Running,
				ActiveSegmentID: "segment", CreatedAt: base, UpdatedAt: updatedAt,
				MessageMark: rundomain.UnknownMessageMark,
			})
			store := &recoveryStoreStub{
				runs:         []rundomain.Run{active},
				transcripts:  map[string][]transcript.Item{},
				messageMarks: map[string]int{"session": 0},
			}
			if !test.itemAt.IsZero() {
				store.transcripts["session"] = []transcript.Item{itemfixture.MustRestore(itemfixture.Input{
					ID: "item", SessionID: "session", RunID: active.ID(),
					Kind: transcript.ToolCall, Status: transcript.ItemRunning,
					OccurredAt: test.itemAt,
					Tool:       &transcript.ToolInvocation{Name: "shell"},
				})}
			}
			if !test.modelAt.IsZero() {
				store.models = []OpenModelInvocation{{
					SessionID: "session", RunID: active.ID(), SegmentID: "segment",
					CallID: "model", StartedAt: test.modelAt,
				}}
			}
			if !test.toolAt.IsZero() {
				store.tools = []OpenToolInvocation{{
					SessionID: "session", RunID: active.ID(), SegmentID: "segment",
					CallID: "tool", ItemID: "tool_item", StartedAt: test.toolAt,
				}}
			}

			recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(
				func(context.Context, WaitingContinuation) (bool, error) { return false, nil },
			))
			if err != nil {
				t.Fatalf("NewRecovery: %v", err)
			}
			recovery.now = func() time.Time { return base.Add(-time.Minute) }

			if _, err := recovery.Reconcile(t.Context()); err != nil {
				t.Fatalf("Reconcile with regressed clock: %v", err)
			}
			if got := store.commit.LostRuns[0].FinishedAt(); !got.Equal(test.want) {
				t.Fatalf("lost Run finish = %v, want durable high watermark %v", got, test.want)
			}
			for _, invocation := range store.commit.ModelInvocations {
				if !invocation.FinishedAt.Equal(test.want) {
					t.Fatalf("model finish = %v, want %v", invocation.FinishedAt, test.want)
				}
			}
			for _, invocation := range store.commit.ToolInvocations {
				if !invocation.FinishedAt.Equal(test.want) {
					t.Fatalf("Tool finish = %v, want %v", invocation.FinishedAt, test.want)
				}
			}
			for _, replacement := range store.commit.ItemReplacements {
				if !replacement.Replacement.FinishedAt().Equal(test.want) {
					t.Fatalf("Item finish = %v, want %v", replacement.Replacement.FinishedAt(), test.want)
				}
			}
		})
	}
}

func TestRecoveryChargesLostGoalOwnedRootToItsAdmissionLease(t *testing.T) {
	createdAt := time.Date(2026, 8, 1, 1, 30, 0, 0, time.UTC)
	finishedAt := createdAt.Add(time.Minute)
	cost := 1.25
	run := runfixture.MustRestore(rundomain.Snapshot{ID: "run_goal", SessionID: "session", State: rundomain.Running,
		ActiveSegmentID: "segment_goal", GoalIncarnationID: "lease_goal",
		Metrics: runfixture.MustMetrics(runfixture.MetricsInput{Steps: 3,
			Usage: &accounting.Usage{Total: accounting.Totals{CostUSD: &cost}}}),

		CreatedAt: createdAt, MessageMark: rundomain.UnknownMessageMark})

	store := &recoveryStoreStub{
		runs:         []rundomain.Run{run},
		transcripts:  map[string][]transcript.Item{run.SessionID(): nil},
		messageMarks: map[string]int{run.SessionID(): 2},
	}
	var notices []invalidation.Notice
	recovery, err := NewRecovery(
		store,
		waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
			return false, nil
		}),
		new(sessionadmission.Gate),
		func(notice invalidation.Notice) { notices = append(notices, notice) },
	)
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}
	recovery.now = func() time.Time { return finishedAt }

	if _, err := recovery.Reconcile(t.Context()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(store.commit.GoalRuns) != 1 {
		t.Fatalf("Goal Runs = %+v, want one", store.commit.GoalRuns)
	}
	goalRun := store.commit.GoalRuns[0]
	if goalRun.SessionID != run.SessionID() || goalRun.IncarnationID != run.GoalIncarnationID() ||
		goalRun.RunID != run.ID() || goalRun.Outcome != rundomain.OutcomeLost ||
		goalRun.CostUSD != cost || goalRun.Steps != run.Metrics().Steps() ||
		!goalRun.CompletedAt.Equal(finishedAt) {
		t.Fatalf("Goal Run = %+v", goalRun)
	}
	wantNotices := []invalidation.Notice{
		invalidation.InSession(invalidation.Runs, run.SessionID(), run.ID()),
		invalidation.InSession(invalidation.Interrupts, run.SessionID(), run.ID()),
		invalidation.InSession(invalidation.Sessions, run.SessionID()),
		invalidation.InSession(invalidation.Goals, run.SessionID()),
	}
	if !reflect.DeepEqual(notices, wantNotices) {
		t.Fatalf("recovery notices = %+v, want %+v", notices, wantNotices)
	}

	missingCharge := store.commit
	missingCharge.GoalRuns = nil
	if err := missingCharge.Validate(); err == nil {
		t.Fatal("RecoveryCommit.Validate accepted a lost goal-owned Run without its charge")
	}
	mismatchedCharge := store.commit
	mismatchedCharge.GoalRuns = append([]goal.RunRecord(nil), store.commit.GoalRuns...)
	mismatchedCharge.GoalRuns[0].IncarnationID = "other-lease"
	if err := mismatchedCharge.Validate(); err == nil {
		t.Fatal("RecoveryCommit.Validate accepted a Goal Run from another incarnation")
	}
	foreignDeletion := store.commit
	foreignDeletion.DeleteInterrupts = append(
		[]InterruptOwner(nil),
		store.commit.DeleteInterrupts...,
	)
	foreignDeletion.DeleteInterrupts = append(foreignDeletion.DeleteInterrupts, InterruptOwner{
		SessionID: "other-session", RootRunID: "run_foreign",
	})
	if err := foreignDeletion.Validate(); err == nil {
		t.Fatal("RecoveryCommit.Validate accepted deletion of an unrelated Pending set")
	}
}

func TestRecoveryPreservesOnlyCoherentInterruptedTree(t *testing.T) {
	run, pending, item := coherentRecoveryPark(t)
	pending.GoalIncarnationID = "goal-lease-1"
	snapshot := run.Snapshot()
	snapshot.GoalIncarnationID = pending.GoalIncarnationID
	run = runfixture.MustRestore(snapshot)
	store := &recoveryStoreStub{
		runs:         []rundomain.Run{run},
		pending:      []Pending{pending},
		transcripts:  map[string][]transcript.Item{run.SessionID(): {item}},
		messageMarks: map[string]int{run.SessionID(): 3},
	}
	var validated WaitingContinuation
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(_ context.Context, continuation WaitingContinuation) (bool, error) {
		validated = continuation
		return true, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	recovered, err := recovery.Reconcile(t.Context())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	wantContinuation, err := waitingContinuationFromPending(pending, ExecutorCheckpoint{
		RootMemberID: "member_root", Payload: []byte(`{}`), BuildID: "test-build",
		Scope: ExecutionScope{
			SessionID: run.SessionID(), CWD: "/workspace", WorkspaceCWD: "/workspace",
			GoalIncarnationID: pending.GoalIncarnationID,
		},
		ModelSelection: run.ModelSelection(), Limits: run.Limits(), Capabilities: pending.Capabilities,
	})
	if err != nil {
		t.Fatalf("waitingContinuationFromPending: %v", err)
	}
	if recovered != 0 || !reflect.DeepEqual(validated, wantContinuation) || len(store.commit.LostRuns) != 0 {
		t.Fatalf("recovery = %d validated=%+v commit=%+v", recovered, validated, store.commit)
	}
	if !reflect.DeepEqual(store.commit.PreservedCheckpointRootIDs, []string{"member_root"}) ||
		len(store.commit.DeleteInterrupts) != 0 {
		t.Fatalf("ownership plan = %+v", store.commit)
	}
}

func TestRecoveryPreservesQuestionToolWhileItsCheckpointIsResumable(t *testing.T) {
	run, pending, questionItem := coherentRecoveryPark(t)
	toolItem := itemfixture.MustRestore(itemfixture.Input{
		ID: "item_tool", SessionID: run.SessionID(), RunID: run.ID(),
		Kind: transcript.ToolCall, Status: transcript.ItemRunning,
		OccurredAt: pending.CreatedAt,
		Tool:       &transcript.ToolInvocation{Name: "ask_user"},
	})
	pending.Continuations[0].DrainedTools = []DrainedTool{{
		ItemID: toolItem.ID(), ItemOccurredAt: toolItem.OccurredAt(),
		CallID: "tool:runtime:0", Name: "ask_user", Arguments: "{}",
	}}
	if err := pending.Validate(); err != nil {
		t.Fatalf("Pending fixture: %v", err)
	}
	store := &recoveryStoreStub{
		runs:        []rundomain.Run{run},
		pending:     []Pending{pending},
		transcripts: map[string][]transcript.Item{run.SessionID(): {questionItem, toolItem}},
	}
	checkpointCalls := 0
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
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
	if recovered != 0 || checkpointCalls != 1 || len(store.commit.LostRuns) != 0 ||
		!reflect.DeepEqual(store.commit.PreservedCheckpointRootIDs, []string{"member_root"}) {
		t.Fatalf("recovery = %d checkpointCalls=%d commit=%+v", recovered, checkpointCalls, store.commit)
	}
}

func TestRecoveryMarksIsolatedParkLostWithoutProbingExecutorCheckpoint(t *testing.T) {
	run, pending, item := coherentRecoveryPark(t)
	store := &recoveryStoreStub{
		runs:        []rundomain.Run{run},
		pending:     []Pending{pending},
		transcripts: map[string][]transcript.Item{run.SessionID(): {item}},
		sessions: map[string]session.Session{
			run.SessionID(): sessionfixture.MustRestore(session.Snapshot{
				ID: run.SessionID(), CWD: "/workspace", Isolated: true,
			}),
		},
	}
	checkpointCalls := 0
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
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
		runs:         []rundomain.Run{run},
		pending:      []Pending{pending},
		transcripts:  map[string][]transcript.Item{run.SessionID(): {item}},
		messageMarks: map[string]int{run.SessionID(): 5},
	}
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
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
		!reflect.DeepEqual(store.commit.DeleteInterrupts, []InterruptOwner{{SessionID: run.SessionID(), RootRunID: run.ID()}}) ||
		len(store.commit.PreservedCheckpointRootIDs) != 0 {
		t.Fatalf("resource-loss recovery = %d, commit %+v", recovered, store.commit)
	}
}

func TestRecoveryTreatsInvalidExecutorCheckpointAsResourceLoss(t *testing.T) {
	run, pending, item := coherentRecoveryPark(t)
	store := &recoveryStoreStub{
		runs:          []rundomain.Run{run},
		pending:       []Pending{pending},
		transcripts:   map[string][]transcript.Item{run.SessionID(): {item}},
		checkpointErr: fmt.Errorf("corrupt durable policy: %w", ErrInvalidExecutorCheckpoint),
	}
	checkpointCalls := 0
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
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
	if recovered != 1 || checkpointCalls != 0 || len(store.commit.LostRuns) != 1 ||
		len(store.commit.PreservedCheckpointRootIDs) != 0 {
		t.Fatalf("invalid-checkpoint recovery = %d checkpointCalls=%d commit=%+v", recovered, checkpointCalls, store.commit)
	}
}

func TestRecoveryAtomicallyClosesLostQuestionToolContext(t *testing.T) {
	run, pending, questionItem := coherentRecoveryPark(t)
	toolItem := itemfixture.MustRestore(itemfixture.Input{
		ID: "item_tool", SessionID: run.SessionID(), RunID: run.ID(),
		Kind: transcript.ToolCall, Status: transcript.ItemRunning,
		OccurredAt: pending.CreatedAt,
		Tool:       &transcript.ToolInvocation{Name: "ask_user"},
	})
	pending.Continuations[0].DrainedTools = []DrainedTool{{
		ItemID: toolItem.ID(), ItemOccurredAt: toolItem.OccurredAt(),
		CallID: "tool:runtime:0", SourceCallID: "provider_call_open",
		Name: "ask_user", Arguments: "{}",
	}}
	conversation := []corechat.Message{
		corechat.NewUserMessage(corechat.NewTextPart("ask me")),
		corechat.NewAssistantMessage(corechat.NewToolCallPart(corechat.ToolCall{
			ID: "provider_call_open", Name: "ask_user", Arguments: "{}",
		})),
	}
	store := &recoveryStoreStub{
		runs:        []rundomain.Run{run},
		pending:     []Pending{pending},
		transcripts: map[string][]transcript.Item{run.SessionID(): {questionItem, toolItem}},
		messages:    map[string][]corechat.Message{run.SessionID(): conversation},
		messageMarks: map[string]int{
			run.SessionID(): len(conversation),
		},
	}
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
		return false, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}
	if _, err := recovery.Reconcile(t.Context()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(store.commit.ConversationTransitions) != 1 || len(store.commit.ItemReplacements) != 1 {
		t.Fatalf("recovery commit = %+v", store.commit)
	}
	transition := store.commit.ConversationTransitions[0]
	if transition.RootRunID != run.ID() || transition.SessionID != run.SessionID() ||
		transition.ExpectedCount != 2 || len(transition.Messages) != 1 {
		t.Fatalf("conversation transition = %+v", transition)
	}
	result := transition.Messages[0].Parts[0].ToolResult
	if result == nil || result.ID != "provider_call_open" || result.Name != "ask_user" ||
		result.Result != recoveryLostToolResult || !result.IsError ||
		store.commit.LostRuns[0].MessageMark() != 3 {
		t.Fatalf("closure/lost Run = %#v / %+v", result, store.commit.LostRuns[0])
	}

	missingClosure := store.commit
	missingClosure.ConversationTransitions = nil
	if err := missingClosure.Validate(); err == nil {
		t.Fatal("RecoveryCommit.Validate accepted a lost tree without its conversation transition")
	}
	wrongWatermark := store.commit
	wrongWatermark.ConversationTransitions = append(
		[]RecoveryConversationTransition(nil),
		store.commit.ConversationTransitions...,
	)
	wrongWatermark.ConversationTransitions[0].ExpectedCount++
	if err := wrongWatermark.Validate(); err == nil {
		t.Fatal("RecoveryCommit.Validate accepted a conversation watermark that differs from the lost Run")
	}
}

func TestRecoveryValidationFailureDoesNotCommitPartialRepair(t *testing.T) {
	run, pending, item := coherentRecoveryPark(t)
	store := &recoveryStoreStub{
		runs:        []rundomain.Run{run},
		pending:     []Pending{pending},
		transcripts: map[string][]transcript.Item{run.SessionID(): {item}},
	}
	want := errors.New("checkpoint backend unavailable")
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
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
		runs:        []rundomain.Run{run},
		pending:     []Pending{pending},
		transcripts: map[string][]transcript.Item{run.SessionID(): {item}},
	}
	checkpointCalls := 0
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
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
		mutate func(*rundomain.Run, *Pending)
	}{
		{
			name: "cumulative metrics",
			mutate: func(_ *rundomain.Run, pending *Pending) {
				metrics, err := rundomain.NewMetrics(nil, pending.Continuations[0].Metrics.Steps()+1, pending.Continuations[0].Metrics.ActiveDuration())
				if err != nil {
					panic(err)
				}
				pending.Continuations[0].Metrics = metrics
			},
		},
		{
			name: "frozen limits",
			mutate: func(_ *rundomain.Run, pending *Pending) {
				pending.Continuations[0].Limits.MaxSteps++
			},
		},
		{
			name: "frozen run capabilities",
			mutate: func(run *rundomain.Run, _ *Pending) {
				snapshot := run.Snapshot()
				snapshot.Capabilities.ChildRuns = true
				*run = runfixture.MustRestore(snapshot)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run, pending, item := coherentRecoveryPark(t)
			test.mutate(&run, &pending)
			store := &recoveryStoreStub{
				runs:        []rundomain.Run{run},
				pending:     []Pending{pending},
				transcripts: map[string][]transcript.Item{run.SessionID(): {item}},
			}
			checkpointCalls := 0
			recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
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
	rootSnapshot := root.Snapshot()
	rootSnapshot.Capabilities.ChildRuns = true
	root = runfixture.MustRestore(rootSnapshot)
	pending.Capabilities.ChildRuns = true
	lineage := rundomain.Lineage{
		SpawnedByItemID: "item_spawn",
		ParentRunID:     root.ID(),
		RootRunID:       root.ID(),
	}
	child := runfixture.MustRestore(rundomain.Snapshot{ID: "run_child", SessionID: root.SessionID(), State: rundomain.Waiting,

		ModelSelection: root.ModelSelection(),
		// This is a valid capabilities in isolation but contradicts the root admission.
		Capabilities: rundomain.Capabilities{
			InterruptKinds: []interruptdomain.Kind{interruptdomain.Question},
		},
		CreatedAt: root.CreatedAt(), MessageMark: rundomain.UnknownMessageMark, Lineage: rundomain.Lineage{SpawnedByItemID: lineage.SpawnedByItemID,
			ParentRunID: lineage.ParentRunID,
			RootRunID:   lineage.RootRunID}})

	rootContinuation := pending.Continuations[0]
	pending.Continuations = []Continuation{
		{
			RunID: "run_child", MemberID: "member_child",
			Lineage: lineage, ModelSelection: root.ModelSelection(),
			RunCreatedAt: root.CreatedAt(),
		},
		rootContinuation,
	}
	store := &recoveryStoreStub{
		runs:        []rundomain.Run{root, child},
		pending:     []Pending{pending},
		transcripts: map[string][]transcript.Item{root.SessionID(): {item}},
	}
	checkpointCalls := 0
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
		checkpointCalls++
		return true, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	if _, err := recovery.Reconcile(t.Context()); err == nil {
		t.Fatal("Reconcile accepted a child Run run capabilities that differs from root admission")
	}
	if store.commits != 0 || checkpointCalls != 0 {
		t.Fatalf("recovery mutated or probed after child policy drift: commits=%d checkpointCalls=%d", store.commits, checkpointCalls)
	}
}

func coherentRecoveryPark(t *testing.T) (rundomain.Run, Pending, transcript.Item) {
	t.Helper()
	createdAt := time.Date(2026, 8, 1, 2, 0, 0, 0, time.UTC)
	selection, err := modelref.New("anthropic", "claude")
	if err != nil {
		t.Fatalf("model selection: %v", err)
	}
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}}
	interrupt := transcript.Interrupt{
		ItemID: "item_question", ItemOccurredAt: createdAt,
		RunID: "run_root", Kind: interruptdomain.Question, Question: question,
	}
	run := runfixture.MustRestore(rundomain.Snapshot{ID: "run_root", SessionID: "session", State: rundomain.Waiting,
		ModelSelection: selection,
		Capabilities:   rundomain.Capabilities{InterruptKinds: []interruptdomain.Kind{interruptdomain.Question}},
		CreatedAt:      createdAt, UpdatedAt: createdAt.Add(time.Second), MessageMark: rundomain.UnknownMessageMark})

	pending := Pending{
		RootRunID:  run.ID(),
		SessionID:  run.SessionID(),
		ExecutorID: "turn_root",
		Interrupts: []transcript.Interrupt{interrupt},
		Capabilities: rundomain.Capabilities{
			InterruptKinds: []interruptdomain.Kind{interruptdomain.Question},
		},
		Bindings: []InterruptBinding{{
			InterruptItemID: interrupt.ItemID,
			MemberID:        "member_root",
			RequestID:       "request_root",
		}},
		Continuations: []Continuation{{
			RunID: run.ID(), MemberID: "member_root", ModelSelection: selection, RunCreatedAt: createdAt,
		}},
		CreatedAt: createdAt.Add(time.Second),
	}
	if err := pending.Validate(); err != nil {
		t.Fatalf("Pending fixture: %v", err)
	}
	item := itemfixture.MustRestore(itemfixture.Input{
		ID: interrupt.ItemID, SessionID: run.SessionID(), RunID: run.ID(),
		Kind:     transcript.QuestionItem,
		Question: question, OccurredAt: interrupt.ItemOccurredAt,
	})
	return run, pending, item
}
