package runsegment

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/itemfixture"
	runfixture "github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
	"github.com/Tangerg/lynx/core/chat"
)

const checkpointBuildID = "test-build"

func claimResumeForTest(
	t *testing.T,
	store *persistence.InterruptStore,
	pending runs.Pending,
) {
	t.Helper()
	answers := make([]runs.InterruptAnswer, len(pending.Bindings))
	for index, binding := range pending.Bindings {
		answers[index] = runs.InterruptAnswer{
			InterruptItemID: binding.InterruptItemID,
			MemberID:        binding.MemberID,
			RequestID:       binding.RequestID,
			Resolution:      interrupt.Resolution{Approved: true},
		}
	}
	_, found, err := store.ClaimResume(
		t.Context(), pending.SessionID, pending.RootRunID, answers, time.Now().UTC(),
	)
	if err != nil || !found {
		t.Fatalf("claim resume for test: found=%t err=%v", found, err)
	}
}

// TestCommitOpeningResumePreservesAnswerClaimOnRollback proves the continuation
// opening leaves its already-claimed hand-off untouched when Run-state
// validation fails.
func TestCommitOpeningResumePreservesAnswerClaimOnRollback(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ints := persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
	history := sqlite.NewTranscriptStore(db)
	state := sqlite.NewRunStore(db)
	ctx := context.Background()
	createdAt := time.Now().UTC()
	if err := state.Admit(ctx, run.Draft{RunID: "run_actual", SessionID: "ses_1", SegmentID: "seg_open", CreatedAt: createdAt}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := state.Suspend(ctx, parkedRunRecord("run_actual", "ses_1", createdAt)); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	stalePending := singleRunPending(
		t,
		"run_stale", "ses_1", "member_stale", "request_stale", "item_stale",
		time.Now().UTC(), time.Now().UTC(),
	)
	if err := ints.Open(ctx, stalePending); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}
	claimResumeForTest(t, ints, stalePending)
	effects := sqliteEffects(sqliteOpeningStores{interrupts: ints, transcript: history}, Config{
		State: state,
		Tx:    func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	resume := run.TreeResumeDraft{
		RootRunID: "run_stale",
		SessionID: "ses_1",
		ResumedAt: time.Now().UTC(),
		Runs:      []run.ResumeDraft{{RunID: "run_stale", SegmentID: "seg_next"}},
	}
	err = effects.CommitOpening(ctx, runs.OpeningCommit{CommitID: "run_commit_stale_resume", Resume: &resume, Events: []runs.EventCommit{{RunID: "run_stale", SessionID: "ses_1", SegmentID: "seg_next"}}})
	if err == nil {
		t.Fatal("CommitOpening must reject an interrupt that does not own the active run")
	}
	if _, found, getErr := ints.Get(ctx, "run_stale"); getErr != nil || found {
		t.Fatalf("rolled-back interrupt found=%v err=%v, want hidden claim", found, getErr)
	}
	if err := ints.RequireResumeClaim(ctx, "ses_1", "run_stale"); err != nil {
		t.Fatalf("rolled-back resume claim: %v", err)
	}
}

func TestCommitOpeningResumeCommitsWholeWriteSet(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ints := persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
	history := sqlite.NewTranscriptStore(db)
	state := sqlite.NewRunStore(db)
	ctx := context.Background()
	created := time.Now().UTC()
	if err := state.Admit(ctx, run.Draft{RunID: "run_1", SessionID: "ses_1", SegmentID: "seg_open", CreatedAt: created}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := state.Suspend(ctx, parkedRunRecord("run_1", "ses_1", created)); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	pending := singleRunPending(
		t,
		"run_1", "ses_1", "member_1", "request_1", "item_question",
		created, time.Now().UTC(),
	)
	if err := ints.Open(ctx, pending); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}
	claimResumeForTest(t, ints, pending)
	effects := sqliteEffects(sqliteOpeningStores{interrupts: ints, transcript: history}, Config{
		State: state,
		Tx:    func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	resume := run.TreeResumeDraft{
		RootRunID: "run_1",
		SessionID: "ses_1",
		ResumedAt: time.Now().UTC(),
		Runs:      []run.ResumeDraft{{RunID: "run_1", SegmentID: "seg_next"}},
	}
	opening := runs.OpeningCommit{
		CommitID: "run_commit_resume",
		Resume:   &resume,
		Events: []runs.EventCommit{{
			RunID:     "run_1",
			SessionID: "ses_1",
			SegmentID: "seg_next",
			Items: []transcript.Item{itemfixture.MustRestore(itemfixture.Input{
				SessionID: "ses_1", RunID: "run_1", ID: "item_resumed", OccurredAt: created,
				Status: transcript.ItemCompleted, Kind: transcript.UserMessage,
				Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "go on"}},
			})},
		}},
	}
	err = effects.CommitOpening(ctx, opening)
	if err != nil {
		t.Fatalf("CommitOpening: %v", err)
	}
	if _, found, getErr := ints.Get(ctx, "run_1"); getErr != nil || found {
		t.Fatalf("interrupt found=%v err=%v, accepted claim must remain hidden", found, getErr)
	}
	if err := ints.RequireResumeClaim(ctx, "ses_1", "run_1"); err != nil {
		t.Fatalf("accepted resume claim: %v", err)
	}
	recorded, listErr := history.List(ctx, "ses_1")
	if listErr != nil || len(recorded) != 1 {
		t.Fatalf("history items=%d err=%v, want the opening projection", len(recorded), listErr)
	}
	var stateName string
	if err := db.QueryRowContext(ctx, `SELECT state FROM runs WHERE run_id = ?`, "run_1").Scan(&stateName); err != nil || stateName != "running" {
		t.Fatalf("run state=%q err=%v, want running", stateName, err)
	}

	if err := effects.CommitOpening(ctx, opening); err != nil {
		t.Fatalf("exact CommitOpening replay = %v, want idempotent success", err)
	}
	recorded, listErr = history.List(ctx, "ses_1")
	if listErr != nil || len(recorded) != 1 {
		t.Fatalf("history after exact replay items=%d err=%v, want unchanged", len(recorded), listErr)
	}
	if err := db.QueryRowContext(ctx, `SELECT state FROM runs WHERE run_id = ?`, "run_1").Scan(&stateName); err != nil || stateName != "running" {
		t.Fatalf("run state after exact replay=%q err=%v, want running", stateName, err)
	}
	opening.CommitID = "run_commit_other_resume"
	if err := effects.CommitOpening(ctx, opening); err == nil {
		t.Fatal("different CommitOpening attempt succeeded after the Run was already resumed")
	}
}

func TestCommitOpeningReconcilesAmbiguousAdmission(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	state := sqlite.NewRunStore(db)
	history := sqlite.NewTranscriptStore(db)
	createdAt := time.Unix(1, 0).UTC()
	draft := run.Draft{
		RunID: "run_ambiguous_opening", SessionID: "ses_ambiguous_opening",
		SegmentID: "seg_ambiguous_opening", CreatedAt: createdAt,
	}
	opening := runs.OpeningCommit{
		CommitID: "run_commit_ambiguous_opening",
		Admit:    &draft,
		Events: []runs.EventCommit{{
			RunID: draft.RunID, SessionID: draft.SessionID, SegmentID: draft.SegmentID,
			Items: []transcript.Item{itemfixture.MustRestore(itemfixture.Input{
				SessionID: draft.SessionID, RunID: draft.RunID, ID: "item_opening",
				OccurredAt: createdAt, Status: transcript.ItemCompleted, Kind: transcript.UserMessage,
				Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}},
			})},
		}},
	}
	commitCtx, cancelCommit := context.WithCancel(ctx)
	t.Cleanup(cancelCommit)
	loseReceipt := true
	effects := New(Config{
		State: state, Transcript: history,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			err := sqlite.RunInTx(ctx, db, fn)
			if err == nil && loseReceipt {
				loseReceipt = false
				cancelCommit()
				return errors.New("lost opening commit receipt")
			}
			return err
		},
	})
	if err := effects.CommitOpening(commitCtx, opening); err != nil {
		t.Fatalf("ambiguous CommitOpening = %v, want reconciled success", err)
	}
	stored, found, err := state.Run(ctx, draft.RunID)
	if err != nil || !found || stored.State() != run.Running || stored.ActiveSegmentID() != draft.SegmentID {
		t.Fatalf("opened Run = %#v found=%t err=%v", stored, found, err)
	}
	matched, err := state.RunCommitCommitted(
		ctx, draft.SessionID, draft.RunID, draft.SegmentID, opening.CommitID,
	)
	if err != nil || !matched {
		t.Fatalf("opening marker matched=%t err=%v, want true/nil", matched, err)
	}
	if err := effects.CommitOpening(commitCtx, opening); err != nil {
		t.Fatalf("exact opening replay = %v, want idempotent success", err)
	}
	items, err := history.List(ctx, draft.SessionID)
	if err != nil || len(items) != 1 || items[0].ID() != "item_opening" {
		t.Fatalf("opening items = %#v err=%v, want one item", items, err)
	}
	opening.CommitID = "run_commit_other_opening"
	if err := effects.CommitOpening(commitCtx, opening); err == nil {
		t.Fatal("different opening attempt reconciled against prior marker")
	}
	requireSQLiteHealthy(t, ctx, db)
}

func TestCommitEventAtomicallyRecordsModelFinalAndRunAccounting(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	startedAt := time.Date(2026, 8, 8, 1, 2, 3, 0, time.UTC)
	finishedAt := startedAt.Add(time.Second)
	runState := sqlite.NewRunStore(db)
	if err := runState.Admit(ctx, run.Draft{
		RunID: "run_model", SessionID: "ses_model", SegmentID: "seg_model", CreatedAt: startedAt,
	}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	history := sqlite.NewTranscriptStore(db)
	invocations := sqlite.NewModelInvocationStore(db)
	effects := New(Config{
		Transcript:       history,
		ModelInvocations: invocations,
		State:            runState,
		RunMetrics:       runState,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	})
	start := runs.ModelInvocationCommit{
		CallID: "model_call_1", SegmentID: "seg_model",
		State: runs.ModelInvocationStarted, StartedAt: startedAt,
	}
	if err := effects.CommitEvent(ctx, runs.EventCommit{
		RunID: "run_model", SessionID: "ses_model", SegmentID: "seg_model", CommitID: "event_commit_model_start",
		ModelInvocations: []runs.ModelInvocationCommit{start},
	}); err != nil {
		t.Fatalf("commit start: %v", err)
	}

	item := itemfixture.MustRestore(itemfixture.Input{
		SessionID: "ses_model", RunID: "run_model", ID: "item_final",
		OccurredAt: finishedAt, Status: transcript.ItemCompleted, Kind: transcript.AgentMessage,
		Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "answer"}},
	})
	completion := runs.ModelInvocationCommit{
		CallID: "model_call_1", SegmentID: "seg_model", State: runs.ModelInvocationCompleted,
		StartedAt: startedAt, FinishedAt: finishedAt,
	}
	usage := &accounting.Usage{Total: accounting.Totals{InputTokens: 2, OutputTokens: 1}}
	wrongSegment := runs.RunProgressCommit{
		SegmentID: "seg_wrong", Metrics: runfixture.MustMetrics(runfixture.MetricsInput{Steps: 1, Usage: usage}), UpdatedAt: finishedAt,
	}
	err = effects.CommitEvent(ctx, runs.EventCommit{
		RunID: "run_model", SessionID: "ses_model", SegmentID: "seg_model", CommitID: "event_commit_model_wrong",
		Items:            []transcript.Item{item},
		ModelInvocations: []runs.ModelInvocationCommit{completion}, Progress: &wrongSegment,
	})
	if err == nil {
		t.Fatal("commit with a stale segment fence succeeded")
	}
	var invocationState string
	if err := db.QueryRowContext(ctx,
		`SELECT state FROM model_invocations WHERE call_id = ?`, "model_call_1",
	).Scan(&invocationState); err != nil || invocationState != "started" {
		t.Fatalf("invocation after rollback = %q err=%v, want started", invocationState, err)
	}
	if items, err := history.List(ctx, "ses_model"); err != nil || len(items) != 0 {
		t.Fatalf("history after rollback = %#v err=%v, want empty", items, err)
	}

	progress := wrongSegment
	progress.SegmentID = "seg_model"
	if err := effects.CommitEvent(ctx, runs.EventCommit{
		RunID: "run_model", SessionID: "ses_model", SegmentID: "seg_model", CommitID: "event_commit_model_complete",
		Items:            []transcript.Item{item},
		ModelInvocations: []runs.ModelInvocationCommit{completion}, Progress: &progress,
	}); err != nil {
		t.Fatalf("commit final: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT state FROM model_invocations WHERE call_id = ?`, "model_call_1",
	).Scan(&invocationState); err != nil || invocationState != "completed" {
		t.Fatalf("invocation after commit = %q err=%v, want completed", invocationState, err)
	}
	recorded, err := history.List(ctx, "ses_model")
	if err != nil || len(recorded) != 1 || recorded[0].ID() != item.ID() {
		t.Fatalf("history after commit = %#v err=%v", recorded, err)
	}
	persistedRun, found, err := runState.Run(ctx, "run_model")
	persistedUsage, reported := persistedRun.Metrics().Usage()
	if err != nil || !found || persistedRun.Metrics().Steps() != 1 || !reported ||
		persistedUsage.Total.InputTokens != 2 || persistedUsage.Total.OutputTokens != 1 {
		t.Fatalf("Run accounting after commit = %#v found=%v err=%v", persistedRun.Metrics(), found, err)
	}
}

func TestCommitEventRejectsTerminalFromReplacedSegment(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	startedAt := time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC)
	parkedAt := startedAt.Add(time.Second)
	resumedAt := parkedAt.Add(time.Second)
	finishedAt := resumedAt.Add(time.Second)
	store := sqlite.NewRunStore(db)
	draft := run.Draft{
		RunID: "run_resumed", SessionID: "ses_resumed", SegmentID: "seg_old", CreatedAt: startedAt,
	}
	if err := store.Admit(ctx, draft); err != nil {
		t.Fatal(err)
	}
	oldSegment, err := run.Admit(draft)
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := oldSegment.Suspend(parkedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Suspend(ctx, waiting); err != nil {
		t.Fatal(err)
	}
	if err := store.Resume(ctx, draft.SessionID, run.ResumeDraft{
		RunID: draft.RunID, SegmentID: "seg_new",
	}, resumedAt); err != nil {
		t.Fatal(err)
	}
	staleTerminal, err := oldSegment.Terminate(run.Termination{
		Outcome: run.OutcomeCompleted, FinishedAt: finishedAt, MessageMark: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	effects := New(Config{
		State: store,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	})
	if err := effects.CommitEvent(ctx, runs.EventCommit{
		RunID: draft.RunID, SessionID: draft.SessionID,
		SegmentID: "seg_old", CommitID: "event_commit_old_segment",
		State: runs.StateTerminalize, Outcome: run.OutcomeCompleted, Run: &staleTerminal,
	}); err == nil {
		t.Fatal("terminal fact from the replaced Segment ended the resumed Run")
	}
	current, found, err := store.Run(ctx, draft.RunID)
	if err != nil || !found {
		t.Fatalf("read resumed Run = found %v, err %v", found, err)
	}
	if current.State() != run.Running || current.ActiveSegmentID() != "seg_new" {
		t.Fatalf("Run after stale terminal = %s/%q, want running seg_new", current.State(), current.ActiveSegmentID())
	}
}

func TestCommitEventRejectsProjectionFromReplacedSegmentBeforeWritingAnything(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	startedAt := time.Date(2026, 8, 15, 2, 0, 0, 0, time.UTC)
	store := sqlite.NewRunStore(db)
	draft := run.Draft{
		RunID: "run_resumed", SessionID: "ses_resumed", SegmentID: "seg_old", CreatedAt: startedAt,
	}
	if err := store.Admit(ctx, draft); err != nil {
		t.Fatal(err)
	}
	oldSegment, err := run.Admit(draft)
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := oldSegment.Suspend(startedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Suspend(ctx, waiting); err != nil {
		t.Fatal(err)
	}
	if err := store.Resume(ctx, draft.SessionID, run.ResumeDraft{
		RunID: draft.RunID, SegmentID: "seg_new",
	}, startedAt.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	history := sqlite.NewTranscriptStore(db)
	messages := sqlite.NewMessageStore(db)
	effects := New(Config{
		State: store, Transcript: history, Conversation: messages,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	})
	item := itemfixture.MustRestore(itemfixture.Input{
		SessionID: draft.SessionID, RunID: draft.RunID, ID: "item_stale",
		Status: transcript.ItemCompleted, Kind: transcript.AgentMessage,
		OccurredAt: startedAt.Add(3 * time.Second),
		Content:    []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "stale"}},
	})
	err = effects.CommitEvent(ctx, runs.EventCommit{
		RunID: draft.RunID, SessionID: draft.SessionID, SegmentID: "seg_old", CommitID: "event_commit_stale_item",
		Items: []transcript.Item{item},
		ConversationMessages: []chat.Message{
			chat.NewAssistantMessage(chat.NewTextPart("stale")),
		},
	})
	if err == nil {
		t.Fatal("projection from the replaced Segment committed")
	}
	if items, listErr := history.List(ctx, draft.SessionID); listErr != nil || len(items) != 0 {
		t.Fatalf("transcript after stale write-set = %#v, %v; want empty", items, listErr)
	}
	if count, countErr := messages.Count(ctx, draft.SessionID); countErr != nil || count != 0 {
		t.Fatalf("conversation after stale write-set = %d, %v; want empty", count, countErr)
	}
	current, found, readErr := store.Run(ctx, draft.RunID)
	if readErr != nil || !found || current.State() != run.Running || current.ActiveSegmentID() != "seg_new" {
		t.Fatalf("Run after stale write-set = found:%t state:%s Segment:%q err:%v", found, current.State(), current.ActiveSegmentID(), readErr)
	}
}

func TestCommitEventAtomicallyRecordsCanonicalToolBatch(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	startedAt := time.Date(2026, 8, 8, 4, 5, 6, 0, time.UTC)
	finishedAt := startedAt.Add(time.Second)
	runState := sqlite.NewRunStore(db)
	if err := runState.Admit(ctx, run.Draft{
		RunID: "run_tools", SessionID: "ses_tools", SegmentID: "seg_tools", CreatedAt: startedAt,
	}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	history := sqlite.NewTranscriptStore(db)
	invocations := sqlite.NewToolInvocationStore(db)
	effects := New(Config{
		Transcript: history, ToolInvocations: invocations, State: runState, RunMetrics: runState,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	})
	starts := []runs.ToolInvocationCommit{
		{CallID: "tool_first", ItemID: "item_first", SegmentID: "seg_tools", State: runs.ToolInvocationStarted, StartedAt: startedAt},
		{CallID: "tool_second", ItemID: "item_second", SegmentID: "seg_tools", State: runs.ToolInvocationStarted, StartedAt: startedAt},
	}
	for index, start := range starts {
		name := []string{"first", "second"}[index]
		running := itemfixture.MustRestore(itemfixture.Input{
			SessionID: "ses_tools", RunID: "run_tools", ID: start.ItemID,
			OccurredAt: startedAt, Status: transcript.ItemRunning, Kind: transcript.ToolCall,
			Tool:        &transcript.ToolInvocation{Name: name, Arguments: tool.Arguments{}},
			SafetyClass: tool.SafetyClassSafe,
		})
		if err := effects.CommitEvent(ctx, runs.EventCommit{
			RunID: "run_tools", SessionID: "ses_tools", SegmentID: "seg_tools", CommitID: "event_commit_tool_start_" + start.CallID,
			Items:           []transcript.Item{running},
			ToolInvocations: []runs.ToolInvocationCommit{start},
		}); err != nil {
			t.Fatalf("commit Tool start %q: %v", start.CallID, err)
		}
	}

	items := make([]transcript.Item, 0, 2)
	terminals := make([]runs.ToolInvocationCommit, 0, 2)
	for index, start := range starts {
		name := []string{"first", "second"}[index]
		result := tool.StringResult(name + "-result")
		items = append(items, itemfixture.MustRestore(itemfixture.Input{
			SessionID: "ses_tools", RunID: "run_tools", ID: start.ItemID,
			OccurredAt: startedAt, FinishedAt: finishedAt,
			Status: transcript.ItemCompleted, Kind: transcript.ToolCall,
			Tool:        &transcript.ToolInvocation{Name: name, Arguments: tool.Arguments{}, Result: &result},
			SafetyClass: tool.SafetyClassSafe,
		}))
		terminals = append(terminals, runs.ToolInvocationCommit{
			CallID: start.CallID, ItemID: start.ItemID, SegmentID: start.SegmentID,
			State: runs.ToolInvocationCompleted, StartedAt: startedAt, FinishedAt: finishedAt,
		})
	}
	wrongSegment := runs.RunProgressCommit{
		SegmentID: "seg_wrong", Metrics: run.Metrics{}, UpdatedAt: finishedAt,
	}
	err = effects.CommitEvent(ctx, runs.EventCommit{
		RunID: "run_tools", SessionID: "ses_tools", SegmentID: "seg_tools", CommitID: "event_commit_tools_wrong",
		Items:           items,
		ToolInvocations: terminals, Progress: &wrongSegment,
	})
	if err == nil {
		t.Fatal("canonical Tool batch with a stale segment fence succeeded")
	}
	if recorded, err := history.List(ctx, "ses_tools"); err != nil || len(recorded) != 2 ||
		recorded[0].ID() != "item_first" || recorded[0].Status() != transcript.ItemRunning ||
		recorded[1].ID() != "item_second" || recorded[1].Status() != transcript.ItemRunning {
		t.Fatalf("history after rollback = %#v err=%v, want the two committed running Items", recorded, err)
	}
	for _, callID := range []string{"tool_first", "tool_second"} {
		var state string
		if err := db.QueryRowContext(ctx,
			`SELECT state FROM tool_invocations WHERE call_id = ?`, callID,
		).Scan(&state); err != nil || state != "started" {
			t.Fatalf("Tool invocation %q after rollback = %q err=%v, want started", callID, state, err)
		}
	}

	progress := wrongSegment
	progress.SegmentID = "seg_tools"
	if err := effects.CommitEvent(ctx, runs.EventCommit{
		RunID: "run_tools", SessionID: "ses_tools", SegmentID: "seg_tools", CommitID: "event_commit_tools_complete",
		Items:           items,
		ToolInvocations: terminals, Progress: &progress,
	}); err != nil {
		t.Fatalf("commit canonical Tool batch: %v", err)
	}
	recorded, err := history.List(ctx, "ses_tools")
	if err != nil || len(recorded) != 2 || recorded[0].ID() != "item_first" || recorded[1].ID() != "item_second" {
		t.Fatalf("canonical history = %#v err=%v", recorded, err)
	}
	for _, callID := range []string{"tool_first", "tool_second"} {
		var state string
		if err := db.QueryRowContext(ctx,
			`SELECT state FROM tool_invocations WHERE call_id = ?`, callID,
		).Scan(&state); err != nil || state != "completed" {
			t.Fatalf("Tool invocation %q after commit = %q err=%v, want completed", callID, state, err)
		}
	}
}

func TestCommitOpeningResumesRunTreeAtomically(t *testing.T) {
	for _, test := range []struct {
		name        string
		suspendRoot bool
		wantError   bool
	}{
		{name: "complete tree", suspendRoot: true},
		{name: "rollback after descendant resumed", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newOpeningResumeFixture(t, test.suspendRoot)
			err := fixture.effects.CommitOpening(
				fixture.ctx,
				runs.OpeningCommit{CommitID: "run_commit_tree_resume", Resume: &fixture.resume},
			)
			if test.wantError {
				assertOpeningResumeRollback(t, fixture, err)
				return
			}
			assertOpeningResumeCommit(t, fixture, err)
		})
	}
}

type openingResumeFixture struct {
	ctx        context.Context
	database   *sql.DB
	interrupts *persistence.InterruptStore
	effects    *Effects
	resume     run.TreeResumeDraft
}

func newOpeningResumeFixture(t *testing.T, suspendRoot bool) openingResumeFixture {
	t.Helper()
	database, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := t.Context()
	runStore := sqlite.NewRunStore(database)
	interruptStore := persistence.NewInterruptStore(sqlite.NewInterruptStore(database))
	createdAt := time.Now().UTC()
	if err := runStore.Admit(ctx, run.Draft{
		RunID: "run_root", SessionID: "session_1", SegmentID: "segment_root", CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("admit root: %v", err)
	}
	lineage := run.Lineage{
		SpawnedByItemID: "item_spawn_child", ParentRunID: "run_root", RootRunID: "run_root",
	}
	if err := runStore.Admit(ctx, run.Draft{
		RunID: "run_child", SessionID: "session_1", SegmentID: "segment_child",
		SpawnedByItemID: lineage.SpawnedByItemID, ParentRunID: lineage.ParentRunID,
		RootRunID: lineage.RootRunID, CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("admit child: %v", err)
	}
	childInterrupts := []transcript.Interrupt{treeQuestion("item_child", "run_child")}
	childRun := waitingTestSessionRun(
		"run_child",
		lineage,
		createdAt,
		childInterrupts,
	)

	if err := runStore.Suspend(ctx, childRun); err != nil {
		t.Fatalf("suspend child: %v", err)
	}
	if suspendRoot {
		rootRun := waitingTestSessionRun("run_root", run.Lineage{}, createdAt, nil)
		if err := runStore.Suspend(ctx, rootRun); err != nil {
			t.Fatalf("suspend root: %v", err)
		}
	}
	pending := runs.Pending{
		RootRunID: "run_root", SessionID: "session_1", ExecutorID: "turn_1",
		Capabilities: run.Capabilities{
			ChildRuns: true, InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		Interrupts: childInterrupts,
		Bindings: []runs.InterruptBinding{{
			InterruptItemID: "item_child", MemberID: "member_child", RequestID: "request_child",
		}},
		Continuations: []runs.Continuation{
			{RunID: "run_child", MemberID: "member_child", Lineage: lineage, RunCreatedAt: createdAt},
			{RunID: "run_root", MemberID: "member_root", RunCreatedAt: createdAt},
		},
		CreatedAt: createdAt.Add(time.Second),
	}
	if err := interruptStore.Open(ctx, pending); err != nil {
		t.Fatalf("put pending: %v", err)
	}
	claimResumeForTest(t, interruptStore, pending)
	effects := sqliteEffects(sqliteOpeningStores{interrupts: interruptStore}, Config{
		State: runStore,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, database, fn)
		},
	})
	return openingResumeFixture{
		ctx: ctx, database: database, interrupts: interruptStore, effects: effects,
		resume: run.TreeResumeDraft{
			RootRunID: "run_root", SessionID: "session_1", ResumedAt: time.Now().UTC(),
			Runs: []run.ResumeDraft{
				{RunID: "run_child", SegmentID: "segment_child_resumed"},
				{RunID: "run_root", SegmentID: "segment_root_resumed"},
			},
		},
	}
}

func assertOpeningResumeRollback(t *testing.T, fixture openingResumeFixture, commitError error) {
	t.Helper()
	if commitError == nil {
		t.Fatal("CommitOpening succeeded with a root Run that was not waiting")
	}
	assertStoredRunState(t, fixture.database, "run_child", "waiting")
	assertStoredRunState(t, fixture.database, "run_root", "running")
	assertResumeClaimRemainsHidden(t, fixture)
}

func assertOpeningResumeCommit(t *testing.T, fixture openingResumeFixture, commitError error) {
	t.Helper()
	if commitError != nil {
		t.Fatalf("CommitOpening: %v", commitError)
	}
	assertStoredRunState(t, fixture.database, "run_child", "running")
	assertStoredRunState(t, fixture.database, "run_root", "running")
	assertResumeClaimRemainsHidden(t, fixture)
}

func assertResumeClaimRemainsHidden(t *testing.T, fixture openingResumeFixture) {
	t.Helper()
	if _, found, err := fixture.interrupts.Get(fixture.ctx, "run_root"); err != nil || found {
		t.Fatalf("pending after opening found=%v err=%v, want hidden claim", found, err)
	}
	if err := fixture.interrupts.RequireResumeClaim(fixture.ctx, "session_1", "run_root"); err != nil {
		t.Fatalf("resume claim after opening: %v", err)
	}
}

func treeQuestion(itemID, runID string) transcript.Interrupt {
	return transcript.Interrupt{
		ItemID: itemID, ItemOccurredAt: time.Unix(1, 0).UTC(),
		RunID: runID, Kind: interrupt.Question,
		Question: &transcript.Question{
			Fields: []transcript.QuestionField{{
				Prompt: "Continue?", Kind: transcript.QuestionText,
			}},
		},
	}
}

func waitingTestSessionRun(
	runID string,
	lineage run.Lineage,
	createdAt time.Time,
	open []transcript.Interrupt,
) run.Run {
	return runfixture.MustRestore(run.Snapshot{ID: runID,
		SessionID: "session_1",

		State: run.Waiting,

		CreatedAt:   createdAt,
		MessageMark: run.UnknownMessageMark, Lineage: run.Lineage{SpawnedByItemID: lineage.SpawnedByItemID,
			ParentRunID: lineage.ParentRunID,
			RootRunID:   lineage.RootRunID}})

}

func assertStoredRunState(t testing.TB, db *sql.DB, runID, want string) {
	t.Helper()
	var got string
	if err := db.QueryRowContext(t.Context(), `SELECT state FROM runs WHERE run_id = ?`, runID).Scan(&got); err != nil {
		t.Fatalf("read Run %q state: %v", runID, err)
	}
	if got != want {
		t.Fatalf("Run %q state = %q, want %q", runID, got, want)
	}
}

// TestCommitOpeningRollsBackScheduledSession proves the occurrence-owned
// Session is not durable until every fresh-opening fact is accepted. In
// particular, a rejected occurrence must not leave a session that can later be
// mistaken for a user-created conversation.
func TestCommitOpeningRollsBackScheduledSession(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	sessions := sqlite.NewSessionStore(db)
	state := sqlite.NewRunStore(db)
	history := sqlite.NewTranscriptStore(db)
	effects := sqliteEffects(sqliteOpeningStores{transcript: history}, Config{
		Sessions:        sessions,
		ScheduleFirings: sqlite.NewScheduleStore(db),
		State:           state,
		Tx:              func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	created := time.Now().UTC()
	draft := run.Draft{RunID: "run_scheduled", SessionID: "ses_scheduled", SegmentID: "seg_open", CreatedAt: created}
	scheduled := sessionfixture.MustRestore(session.Snapshot{
		ID: draft.SessionID, Title: "scheduled", CWD: "/work",
		StartedAt: created, UpdatedAt: created, Revision: 1,
	})
	err = effects.CommitOpening(ctx, runs.OpeningCommit{
		CommitID:       "run_commit_claimed_resume",
		Admit:          &draft,
		InitialSession: &scheduled,
		// No firing is seeded: Accept fails after Insert and Admit, so the
		// test exercises rollback rather than a preflight rejection.
		ScheduleFiring: "fire_missing",
		Events: []runs.EventCommit{{
			RunID: draft.RunID, SessionID: draft.SessionID, SegmentID: draft.SegmentID,
			Run: runPointer(runfixture.MustRestore(run.Snapshot{ID: draft.RunID, SessionID: draft.SessionID, UpdatedAt: created})),
		}},
	})
	if err == nil {
		t.Fatal("CommitOpening accepted an unknown scheduled occurrence")
	}
	if _, getErr := sessions.Get(ctx, draft.SessionID); !errors.Is(getErr, session.ErrNotFound) {
		t.Fatalf("scheduled session survived rejected opening: %v", getErr)
	}
	if err := state.Admit(ctx, draft); err != nil {
		t.Fatalf("rejected opening left run admission behind: %v", err)
	}
}

// TestCommitEventRecordsGoalRunWithTerminalRun proves budget accounting is a
// terminal Run fact, not a best-effort follow-up by the Goal driver. Both the
// Run state and the Goal aggregate must become visible together.
func TestCommitEventRecordsGoalRunWithTerminalRun(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	created := time.Now().UTC()
	goals := sqlite.NewGoalStore(db)
	sessions := sqlite.NewSessionStore(db)
	goalSession := sessionfixture.MustRestore(session.Snapshot{
		ID: "ses_goal", CWD: "/work", StartedAt: created, UpdatedAt: created, Revision: 1,
	})
	if err := sessions.Insert(ctx, goalSession); err != nil {
		t.Fatalf("seed goal session: %v", err)
	}
	selection := mustEffectSelection(t, "provider", "model")
	g, err := goal.New("ses_goal", "finish", selection, goal.Budget{MaxRuns: 1}, run.Capabilities{}, "lease_goal", created)
	if err != nil {
		t.Fatalf("new goal: %v", err)
	}
	if _, saved, err := goals.Save(ctx, g, goal.Version{}); err != nil || !saved {
		t.Fatalf("seed goal saved=%v err=%v", saved, err)
	}
	state := sqlite.NewRunStore(db)
	draft := run.Draft{
		RunID: "run_goal", SessionID: g.SessionID, SegmentID: "seg_open",
		GoalIncarnationID: g.IncarnationID, CreatedAt: created,
	}
	if err := state.Admit(ctx, draft); err != nil {
		t.Fatalf("admit goal run: %v", err)
	}
	history := sqlite.NewTranscriptStore(db)
	effects := sqliteEffects(sqliteOpeningStores{transcript: history}, Config{
		GoalRuns: goals,
		State:    state,
		Tx:       func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	costUSD := 0.25
	updated, err := run.Admit(draft)
	if err != nil {
		t.Fatalf("build admitted Run: %v", err)
	}
	finishedAt := created.Add(time.Second)
	updated, err = updated.AdvanceMetrics(runfixture.MustMetrics(runfixture.MetricsInput{Steps: 2,
		Usage: &accounting.Usage{Total: accounting.Totals{CostUSD: &costUSD}}}), finishedAt)
	if err != nil {
		t.Fatalf("advance Run metrics: %v", err)
	}
	updated, err = updated.Terminate(run.Termination{
		Outcome: run.OutcomeCompleted, FinishedAt: finishedAt, MessageMark: run.UnknownMessageMark,
	})
	if err != nil {
		t.Fatalf("finish Run: %v", err)
	}
	updated = mustResolveMessageMark(t, updated, 0)
	finished := &updated
	if err := effects.CommitEvent(ctx, runs.EventCommit{
		RunID: draft.RunID, SessionID: draft.SessionID, SegmentID: draft.SegmentID, State: runs.StateTerminalize,
		CommitID: "event_commit_goal",
		Outcome:  run.OutcomeCompleted,
		Run:      finished,
		GoalRun: &goal.RunRecord{
			SessionID: g.SessionID, IncarnationID: g.IncarnationID, RunID: draft.RunID,
			Outcome: run.OutcomeCompleted, CostUSD: costUSD, Steps: 2, CompletedAt: finished.FinishedAt(),
		},
	}); err != nil {
		t.Fatalf("CommitEvent: %v", err)
	}
	got, found, err := goals.Get(ctx, g.SessionID)
	if err != nil || !found {
		t.Fatalf("goal after terminal found=%v err=%v", found, err)
	}
	if got.Used != (goal.Usage{Runs: 1, CostUSD: 0.25, Steps: 2}) || got.Status != goal.StatusBlocked || got.Reason.Code != goal.ReasonRunBudgetReached {
		t.Fatalf("goal after terminal = %+v", got)
	}
	var runState string
	if err := db.QueryRowContext(ctx, `SELECT state FROM runs WHERE run_id = ?`, draft.RunID).Scan(&runState); err != nil || runState != "terminal" {
		t.Fatalf("run state=%q err=%v, want terminal", runState, err)
	}
}

// TestCommitTreeBarrierProducesDurableTriplet is the event boundary's
// evidence for parked_tree_has_exactly_one_open_interrupt_set: one tree commit
// lands the pending set, opaque executor checkpoint, and suspended Run together
// while a fresh admission is refused.
func TestCommitTreeBarrierProducesDurableTriplet(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ints := persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
	history := sqlite.NewTranscriptStore(db)
	state := sqlite.NewRunStore(db)
	ctx := t.Context()
	createdAt := time.Unix(1, 0).UTC()
	parkedAt := time.Unix(2, 0).UTC()
	if err := state.Admit(ctx, run.Draft{
		RunID: "run_1", SessionID: "ses_1", SegmentID: "seg_open",
		ModelSelection: mustEffectSelection(t, "anthropic", "claude"),
		Capabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	const rootMemberID = "member_1"
	checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	toolInvocations := sqlite.NewToolInvocationStore(db)
	toolStartedAt := createdAt.Add(500 * time.Millisecond)
	if err := toolInvocations.StartToolInvocation(
		ctx, "ses_1", "run_1", "seg_open", "tool_ask", "item_tool", toolStartedAt,
	); err != nil {
		t.Fatalf("start Tool invocation: %v", err)
	}
	checkpoint := executorCheckpoint(t, rootMemberID, "opaque waiting checkpoint", runs.ExecutorCheckpoint{
		BuildID:        checkpointBuildID,
		Scope:          runs.ExecutionScope{SessionID: "ses_1"},
		ModelSelection: mustEffectSelection(t, "anthropic", "claude"),
		Usage:          accounting.Snapshot{},
	})
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}}
	commitCtx, cancelCommit := context.WithCancel(ctx)
	t.Cleanup(cancelCommit)
	loseReceipt := true
	effects := sqliteEffects(sqliteOpeningStores{interrupts: ints, transcript: history}, Config{
		State:               state,
		ExecutorCheckpoints: checkpointStore,
		ToolInvocations:     toolInvocations,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			err := sqlite.RunInTx(ctx, db, fn)
			if err == nil && loseReceipt {
				loseReceipt = false
				cancelCommit()
				return errors.New("lost tree barrier commit receipt")
			}
			return err
		},
	})
	pending := singleRunPending(
		t,
		"run_1", "ses_1", rootMemberID, "request-member_1", "item_question",
		createdAt, parkedAt,
	)
	pending.Continuations[0].DrainedTools = []runs.DrainedTool{{
		ItemID: "item_tool", ItemOccurredAt: toolStartedAt,
		CallID: "tool_ask", Name: "ask_user", Arguments: "{}",
	}}
	barrier := runs.TreeBarrierCommit{
		CommitID:   "run_commit_durable_barrier",
		Pending:    pending,
		Checkpoint: checkpoint,
		Runs: []runs.EventCommit{{
			RunID: "run_1", SessionID: "ses_1", SegmentID: "seg_open", State: runs.StateSuspend,
			Items: []transcript.Item{
				itemfixture.MustRestore(itemfixture.Input{
					SessionID: "ses_1", ID: "item_tool", RunID: "run_1",
					Status: transcript.ItemRunning, Kind: transcript.ToolCall,
					Tool: &transcript.ToolInvocation{Name: "ask_user"}, OccurredAt: toolStartedAt,
				}),
				itemfixture.MustRestore(itemfixture.Input{
					SessionID: "ses_1", ID: "item_question", RunID: "run_1",
					Status: transcript.ItemCompleted, Kind: transcript.QuestionItem,
					Question: question, OccurredAt: parkedAt,
				}),
			},
			ToolInvocations: []runs.ToolInvocationCommit{{
				CallID: "tool_ask", ItemID: "item_tool", SegmentID: "seg_open",
				State: runs.ToolInvocationIncomplete, StartedAt: toolStartedAt, FinishedAt: parkedAt,
			}},
			Run: runPointer(runfixture.MustRestore(run.Snapshot{SessionID: "ses_1", ID: "run_1", State: run.Waiting,
				ModelSelection: pending.Continuations[0].ModelSelection,
				Capabilities:   pending.Capabilities,
				CreatedAt:      createdAt, UpdatedAt: parkedAt, MessageMark: -1})),
		}},
	}
	if err := effects.CommitTreeBarrier(commitCtx, barrier); err != nil {
		t.Fatalf("park: %v", err)
	}
	if err := effects.CommitTreeBarrier(commitCtx, barrier); err != nil {
		t.Fatalf("exact barrier replay = %v, want idempotent success", err)
	}
	matched, err := state.RunCommitCommitted(
		ctx, "ses_1", "run_1", "seg_open", barrier.CommitID,
	)
	if err != nil || !matched {
		t.Fatalf("barrier marker matched=%t err=%v, want true/nil", matched, err)
	}

	if stored, err := checkpointStore.LoadCheckpoint(ctx, rootMemberID); err != nil || stored.RootMemberID != rootMemberID {
		t.Fatalf("stored executor checkpoint = (%+v, %v)", stored, err)
	}
	var toolState string
	if err := db.QueryRowContext(ctx,
		`SELECT state FROM tool_invocations WHERE call_id = ? AND segment_id = ?`,
		"tool_ask", "seg_open",
	).Scan(&toolState); err != nil || toolState != "incomplete" {
		t.Fatalf("parked Tool invocation state = %q err=%v, want incomplete", toolState, err)
	}
	items, err := history.List(ctx, "ses_1")
	if err != nil || len(items) != 2 || items[0].ID() != "item_tool" ||
		items[0].Status() != transcript.ItemRunning {
		t.Fatalf("parked transcript Items = %+v err=%v, want running Tool then Question", items, err)
	}
	if err := state.Admit(ctx, run.Draft{RunID: "run_next", SessionID: "ses_1", SegmentID: "seg_open", CreatedAt: parkedAt}); !errors.Is(err, run.ErrSessionBusy) {
		t.Fatalf("admit after intact park = %v, want ErrSessionBusy", err)
	}
	requireSQLiteHealthy(t, ctx, db)
}

func TestCommitTreeBarrierRollsBackCheckpointWhenRunSuspendFails(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := t.Context()
	createdAt := time.Unix(1, 0).UTC()
	parkedAt := time.Unix(2, 0).UTC()
	const rootMemberID = "member_rollback"
	checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	checkpoint := executorCheckpoint(t, rootMemberID, "opaque rollback checkpoint", runs.ExecutorCheckpoint{
		BuildID:        checkpointBuildID,
		Scope:          runs.ExecutionScope{SessionID: "ses_rollback"},
		ModelSelection: mustEffectSelection(t, "anthropic", "claude"),
		Usage:          accounting.Snapshot{},
	})
	interruptStore := persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
	effects := sqliteEffects(sqliteOpeningStores{
		interrupts: interruptStore,
		transcript: sqlite.NewTranscriptStore(db),
	}, Config{
		State:               sqlite.NewRunStore(db),
		ExecutorCheckpoints: checkpointStore,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	})
	pending := singleRunPending(
		t,
		"run_missing", "ses_rollback", rootMemberID, "request-"+rootMemberID, "item_question",
		createdAt, parkedAt,
	)
	parkedRun := parkedRunRecord("run_missing", "ses_rollback", createdAt)
	err = effects.CommitTreeBarrier(ctx, runs.TreeBarrierCommit{
		CommitID:   "run_commit_rollback_barrier",
		Pending:    pending,
		Checkpoint: checkpoint,
		Runs: []runs.EventCommit{{
			RunID: "run_missing", SessionID: "ses_rollback", SegmentID: "seg_open", State: runs.StateSuspend,
			Run: &parkedRun,
		}},
	})
	if err == nil {
		t.Fatal("CommitTreeBarrier succeeded without an admitted Run")
	}
	if _, loadErr := checkpointStore.LoadCheckpoint(ctx, rootMemberID); !errors.Is(loadErr, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("checkpoint survived failed tree barrier: %v", loadErr)
	}
	if _, found, getErr := interruptStore.Get(ctx, pending.RootRunID); getErr != nil || found {
		t.Fatalf("interrupt survived failed tree barrier: found=%v err=%v", found, getErr)
	}
}

func TestClaimResumeAtomicallyRecordsAnswerAndInvalidatesCheckpoint(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	createdAt := time.Unix(1, 0).UTC()
	claimedAt := createdAt.Add(2 * time.Second)
	interruptStore := persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
	checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	transcriptStore := sqlite.NewTranscriptStore(db)
	pending := singleRunPending(
		t,
		"run_claim", "session_claim", "member_claim", "request_claim", "item_claim",
		createdAt, createdAt.Add(time.Second),
	)
	if err := interruptStore.Open(ctx, pending); err != nil {
		t.Fatalf("open interrupt: %v", err)
	}
	questionItem, err := transcript.NewQuestion(transcript.ItemIdentity{
		SessionID:  pending.SessionID,
		RunID:      pending.Interrupts[0].RunID,
		ItemID:     pending.Interrupts[0].ItemID,
		OccurredAt: pending.Interrupts[0].ItemOccurredAt,
	}, *pending.Interrupts[0].Question)
	if err != nil {
		t.Fatalf("new question Item: %v", err)
	}
	if err := transcriptStore.AppendItem(ctx, questionItem); err != nil {
		t.Fatalf("seed question Item: %v", err)
	}
	root, _ := pending.RootContinuation()
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID:   root.MemberID,
		Payload:        []byte(`{"opaque":"tree"}`),
		BuildID:        checkpointBuildID,
		Scope:          runs.ExecutionScope{SessionID: pending.SessionID},
		ModelSelection: root.ModelSelection,
		Limits:         root.Limits,
	}
	if err := checkpointStore.SaveCheckpoint(ctx, checkpoint); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}
	runStore := sqlite.NewRunStore(db)
	if err := runStore.Admit(ctx, run.Draft{
		RunID: pending.RootRunID, SessionID: pending.SessionID, SegmentID: "segment_claim",
		ModelSelection: root.ModelSelection, GoalIncarnationID: pending.GoalIncarnationID,
		Limits: root.Limits, Capabilities: pending.Capabilities, CreatedAt: root.RunCreatedAt,
	}); err != nil {
		t.Fatalf("admit claim root Run: %v", err)
	}
	runningRoot, found, err := runStore.Run(ctx, pending.RootRunID)
	if err != nil || !found {
		t.Fatalf("read claim root Run: found=%t err=%v", found, err)
	}
	waitingRoot, err := runningRoot.Suspend(pending.CreatedAt)
	if err != nil {
		t.Fatalf("park claim root Run: %v", err)
	}
	if err := runStore.Suspend(ctx, waitingRoot); err != nil {
		t.Fatalf("persist claim root park: %v", err)
	}
	effects := New(Config{
		Interrupts:          interruptStore,
		ResumeClaims:        interruptStore,
		ExecutorCheckpoints: checkpointStore,
		ItemReplacer:        transcriptStore,
		State:               runStore,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	})
	answers := []runs.InterruptAnswer{{
		InterruptItemID: pending.Bindings[0].InterruptItemID,
		MemberID:        pending.Bindings[0].MemberID,
		RequestID:       pending.Bindings[0].RequestID,
		Resolution:      interrupt.Resolution{Answers: [][]string{{"continue"}}},
	}}
	replacements, err := (runs.ResumeClaimCommit{
		CommitID: "run_commit_resume_claim", Expected: pending, Answers: answers, ClaimedAt: claimedAt,
	}).QuestionReplacements()
	if err != nil {
		t.Fatalf("prepare question replacement: %v", err)
	}
	storedQuestion, found, err := transcriptStore.Item(ctx, questionItem.ID())
	if err != nil || !found {
		t.Fatalf("stored question Item = found:%t err:%v", found, err)
	}
	if !reflect.DeepEqual(storedQuestion.Snapshot(), replacements[0].Expected.Snapshot()) {
		storedValue, _ := storedQuestion.Question()
		expectedValue, _ := replacements[0].Expected.Question()
		t.Fatalf("stored/expected question differ:\n stored: %#v\nexpected: %#v\nitems: %#v / %#v", storedValue, expectedValue, storedQuestion.Snapshot(), replacements[0].Expected.Snapshot())
	}

	stale := pending
	stale.ExecutorID = "turn_stale"
	if _, err := effects.ClaimResume(ctx, runs.ResumeClaimCommit{
		CommitID: "run_commit_stale_resume_claim", Expected: stale, Answers: answers, ClaimedAt: claimedAt,
	}); err == nil {
		t.Fatal("ClaimResume accepted a stale waiting hand-off")
	}
	if _, found, err := interruptStore.Get(ctx, pending.RootRunID); err != nil || !found {
		t.Fatalf("interrupt after rolled-back claim = found:%t err:%v", found, err)
	}
	if _, err := checkpointStore.LoadCheckpoint(ctx, root.MemberID); err != nil {
		t.Fatalf("checkpoint after rolled-back claim: %v", err)
	}

	claimed, err := effects.ClaimResume(ctx, runs.ResumeClaimCommit{
		CommitID: "run_commit_resume_claim", Expected: pending, Answers: answers, ClaimedAt: claimedAt,
	})
	if err != nil {
		t.Fatalf("ClaimResume: %v", err)
	}
	if !reflect.DeepEqual(claimed.Pending, pending) || !reflect.DeepEqual(claimed.Answers, answers) ||
		!reflect.DeepEqual(claimed.Checkpoint, checkpoint) {
		t.Fatalf("claimed resume = %+v", claimed)
	}
	if _, found, err := interruptStore.Get(ctx, pending.RootRunID); err != nil || found {
		t.Fatalf("open interrupt after claim = found:%t err:%v, want hidden", found, err)
	}
	if _, err := checkpointStore.LoadCheckpoint(ctx, root.MemberID); !errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("checkpoint after claim = %v, want not found", err)
	}
	answeredItem, found, err := transcriptStore.Item(ctx, questionItem.ID())
	if err != nil || !found {
		t.Fatalf("answered question Item = found:%t err:%v", found, err)
	}
	answeredQuestion, _ := answeredItem.Question()
	if !reflect.DeepEqual(answeredQuestion.Answers, [][]string{{"continue"}}) {
		t.Fatalf("answered question = %+v", answeredQuestion)
	}
	var state, encodedAnswers string
	var storedClaimedAt int64
	if err := db.QueryRowContext(ctx,
		`SELECT state, answers, claimed_at FROM interrupts WHERE root_run_id = ?`,
		pending.RootRunID,
	).Scan(&state, &encodedAnswers, &storedClaimedAt); err != nil {
		t.Fatalf("read answer claim: %v", err)
	}
	if state != "resuming" || storedClaimedAt != claimedAt.UnixNano() ||
		!strings.Contains(encodedAnswers, `"requestId":"request_claim"`) ||
		!strings.Contains(encodedAnswers, `"answers":[["continue"]]`) {
		t.Fatalf("answer claim state=%q answers=%s claimedAt=%d", state, encodedAnswers, storedClaimedAt)
	}

	next := pending
	next.Interrupts[0].ItemID = "item_next"
	next.Interrupts[0].ItemOccurredAt = claimedAt.Add(time.Second)
	next.Bindings[0].InterruptItemID = "item_next"
	next.Bindings[0].RequestID = "request_next"
	next.CreatedAt = claimedAt.Add(time.Second)
	if err := interruptStore.Open(ctx, next); err != nil {
		t.Fatalf("advance to next quiescent barrier: %v", err)
	}
	if got, found, err := interruptStore.Get(ctx, next.RootRunID); err != nil || !found || !reflect.DeepEqual(got, next) {
		t.Fatalf("next interrupt = found:%t value:%+v err:%v", found, got, err)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT state, answers, claimed_at FROM interrupts WHERE root_run_id = ?`,
		next.RootRunID,
	).Scan(&state, &encodedAnswers, &storedClaimedAt); err != nil {
		t.Fatalf("read advanced barrier: %v", err)
	}
	if state != "open" || encodedAnswers != "" || storedClaimedAt != 0 {
		t.Fatalf("advanced barrier state=%q answers=%q claimedAt=%d", state, encodedAnswers, storedClaimedAt)
	}
}

func TestClaimResumeAtomicallyPersistsToolApprovalDecision(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	createdAt := time.Unix(20, 0).UTC()
	pending := singleRunPending(
		t, "run_approval_claim", "session_approval_claim", "member_approval_claim",
		"request_approval_claim", "item_approval_claim", createdAt, createdAt.Add(time.Second),
	)
	arguments, err := tool.ArgumentsFromMap(map[string]any{
		"command": "go test ./...", "description": "Run tests",
	})
	if err != nil {
		t.Fatalf("tool arguments: %v", err)
	}
	invocation := transcript.ToolInvocation{Name: "shell", Arguments: arguments}
	pending.Capabilities.InterruptKinds = []interrupt.Kind{interrupt.Approval}
	pending.Interrupts[0].Kind = interrupt.Approval
	pending.Interrupts[0].Question = nil
	pending.Interrupts[0].Approval = &transcript.Approval{
		Tool: invocation, Risk: tool.RiskHigh,
	}
	pending.Bindings[0].ToolCallID = "call_approval_claim"
	if err := pending.Validate(); err != nil {
		t.Fatalf("approval Pending: %v", err)
	}

	interrupts := persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
	checkpoints := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	transcriptStore := sqlite.NewTranscriptStore(db)
	runStore := sqlite.NewRunStore(db)
	if err := interrupts.Open(ctx, pending); err != nil {
		t.Fatalf("open interrupt: %v", err)
	}
	toolItem, err := transcript.NewToolCall(transcript.ItemIdentity{
		SessionID: pending.SessionID, RunID: pending.RootRunID,
		ItemID: pending.Interrupts[0].ItemID, OccurredAt: pending.Interrupts[0].ItemOccurredAt,
	}, invocation, tool.SafetyClassExec)
	if err != nil {
		t.Fatalf("new ToolCall: %v", err)
	}
	if err := transcriptStore.AppendItem(ctx, toolItem); err != nil {
		t.Fatalf("seed ToolCall: %v", err)
	}
	root, _ := pending.RootContinuation()
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID: root.MemberID, Payload: []byte(`{"opaque":"tree"}`),
		BuildID: checkpointBuildID, Scope: runs.ExecutionScope{SessionID: pending.SessionID},
		ModelSelection: root.ModelSelection, Limits: root.Limits,
	}
	if err := checkpoints.SaveCheckpoint(ctx, checkpoint); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}
	if err := runStore.Admit(ctx, run.Draft{
		RunID: pending.RootRunID, SessionID: pending.SessionID, SegmentID: "segment_approval_claim",
		ModelSelection: root.ModelSelection, Limits: root.Limits,
		Capabilities: pending.Capabilities, CreatedAt: root.RunCreatedAt,
	}); err != nil {
		t.Fatalf("admit Run: %v", err)
	}
	runningRoot, found, err := runStore.Run(ctx, pending.RootRunID)
	if err != nil || !found {
		t.Fatalf("read Run: found=%t err=%v", found, err)
	}
	waitingRoot, err := runningRoot.Suspend(pending.CreatedAt)
	if err != nil {
		t.Fatalf("park Run: %v", err)
	}
	if err := runStore.Suspend(ctx, waitingRoot); err != nil {
		t.Fatalf("persist Run park: %v", err)
	}
	answer := runs.InterruptAnswer{
		InterruptItemID: pending.Bindings[0].InterruptItemID,
		MemberID:        pending.Bindings[0].MemberID, RequestID: pending.Bindings[0].RequestID,
		Resolution: interrupt.Resolution{Approved: true},
	}
	claim := runs.ResumeClaimCommit{
		CommitID: "run_commit_approval_claim", Expected: pending,
		Answers: []runs.InterruptAnswer{answer}, ClaimedAt: pending.CreatedAt.Add(time.Second),
	}
	newEffects := func(state RunStore) *Effects {
		return New(Config{
			ResumeClaims: interrupts, ExecutorCheckpoints: checkpoints,
			ToolApprovals: transcriptStore, State: state,
			Tx: func(ctx context.Context, fn func(context.Context) error) error {
				return sqlite.RunInTx(ctx, db, fn)
			},
		})
	}

	markerErr := errors.New("write approval claim marker")
	if _, err := newEffects(failingWaitingRunWriter{
		RunStore: runStore, recordErr: markerErr,
	}).ClaimResume(ctx, claim); !errors.Is(err, markerErr) {
		t.Fatalf("ClaimResume rollback error = %v, want marker failure", err)
	}
	rolledBack, found, err := transcriptStore.Item(ctx, toolItem.ID())
	if err != nil || !found || rolledBack.ApprovalDecision() != "" {
		t.Fatalf("ToolCall after rollback = found:%t decision:%q err:%v", found, rolledBack.ApprovalDecision(), err)
	}
	if _, found, err := interrupts.Get(ctx, pending.RootRunID); err != nil || !found {
		t.Fatalf("Pending after rollback = found:%t err:%v", found, err)
	}
	if _, err := checkpoints.LoadCheckpoint(ctx, root.MemberID); err != nil {
		t.Fatalf("checkpoint after rollback: %v", err)
	}

	if _, err := newEffects(runStore).ClaimResume(ctx, claim); err != nil {
		t.Fatalf("ClaimResume: %v", err)
	}
	resolved, found, err := transcriptStore.Item(ctx, toolItem.ID())
	if err != nil || !found || resolved.ApprovalDecision() != approval.Allow {
		t.Fatalf("resolved ToolCall = found:%t decision:%q err:%v", found, resolved.ApprovalDecision(), err)
	}
	if _, found, err := interrupts.Get(ctx, pending.RootRunID); err != nil || found {
		t.Fatalf("Pending after commit = found:%t err:%v", found, err)
	}
	if _, err := checkpoints.LoadCheckpoint(ctx, root.MemberID); !errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("checkpoint after commit = %v, want not found", err)
	}
	requireSQLiteHealthy(t, ctx, db)
}

func TestClaimResumeReconcilesAmbiguousCommit(t *testing.T) {
	fixture := newResumeClaimSQLiteFixture(t, "ambiguous")
	receiptErr := errors.New("lost resume claim commit receipt")
	commitCtx, cancelCommit := context.WithCancel(fixture.ctx)
	t.Cleanup(cancelCommit)
	effects := fixture.effects(func(ctx context.Context, fn func(context.Context) error) error {
		if err := sqlite.RunInTx(ctx, fixture.db, fn); err != nil {
			return err
		}
		cancelCommit()
		return receiptErr
	}, fixture.runStore)

	claimed, err := effects.ClaimResume(commitCtx, fixture.claim)
	if err != nil {
		t.Fatalf("ClaimResume after lost COMMIT receipt: %v", err)
	}
	if !reflect.DeepEqual(claimed.Pending, fixture.pending) ||
		!reflect.DeepEqual(claimed.Answers, fixture.answers) ||
		!reflect.DeepEqual(claimed.Checkpoint, fixture.checkpoint) {
		t.Fatalf("reconciled claim = %+v", claimed)
	}
	matched, err := fixture.runStore.RunCommitCommitted(
		fixture.ctx,
		fixture.pending.SessionID,
		fixture.pending.RootRunID,
		"",
		fixture.claim.CommitID,
	)
	if err != nil || !matched {
		t.Fatalf("resume claim commit marker = %t err=%v, want exact match", matched, err)
	}
	matched, err = fixture.runStore.RunCommitCommitted(
		fixture.ctx,
		fixture.pending.SessionID,
		fixture.pending.RootRunID,
		"",
		fixture.claim.CommitID+"_other",
	)
	if err != nil || matched {
		t.Fatalf("different resume claim commit marker = %t err=%v, want no match", matched, err)
	}
	assertResumeClaimCommitted(t, fixture)
	requireSQLiteHealthy(t, fixture.ctx, fixture.db)
}

func TestClaimResumeRollsBackWhenCommitMarkerFails(t *testing.T) {
	fixture := newResumeClaimSQLiteFixture(t, "marker_rollback")
	markerErr := errors.New("write resume claim commit marker")
	effects := fixture.effects(
		func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, fixture.db, fn)
		},
		failingWaitingRunWriter{RunStore: fixture.runStore, recordErr: markerErr},
	)

	if _, err := effects.ClaimResume(fixture.ctx, fixture.claim); !errors.Is(err, markerErr) {
		t.Fatalf("ClaimResume error = %v, want marker failure", err)
	}
	gotPending, found, err := fixture.interrupts.Get(fixture.ctx, fixture.pending.RootRunID)
	if err != nil || !found || !reflect.DeepEqual(gotPending, fixture.pending) {
		t.Fatalf("interrupt after marker rollback = found:%t value:%+v err:%v", found, gotPending, err)
	}
	gotCheckpoint, err := fixture.checkpoints.LoadCheckpoint(fixture.ctx, fixture.checkpoint.RootMemberID)
	if err != nil || !executorCheckpointValuesEqual(gotCheckpoint, fixture.checkpoint) {
		t.Fatalf("checkpoint after marker rollback = %+v err=%v", gotCheckpoint, err)
	}
	gotQuestion, found, err := fixture.transcript.Item(fixture.ctx, fixture.question.ID())
	if err != nil || !found {
		t.Fatalf("question after marker rollback = found:%t err:%v", found, err)
	}
	question, ok := gotQuestion.Question()
	if !ok || len(question.Answers) != 0 {
		t.Fatalf("question after marker rollback = %+v, want unanswered", question)
	}
	matched, err := fixture.runStore.RunCommitCommitted(
		fixture.ctx,
		fixture.pending.SessionID,
		fixture.pending.RootRunID,
		"",
		fixture.claim.CommitID,
	)
	if err != nil || matched {
		t.Fatalf("resume claim marker after rollback = %t err=%v, want absent", matched, err)
	}
	requireSQLiteHealthy(t, fixture.ctx, fixture.db)
}

type resumeClaimSQLiteFixture struct {
	ctx         context.Context
	db          *sql.DB
	pending     runs.Pending
	answers     []runs.InterruptAnswer
	claim       runs.ResumeClaimCommit
	checkpoint  runs.ExecutorCheckpoint
	question    transcript.Item
	interrupts  *persistence.InterruptStore
	checkpoints *persistence.ExecutorCheckpointStore
	transcript  *sqlite.TranscriptStore
	runStore    *sqlite.RunStore
}

func newResumeClaimSQLiteFixture(t *testing.T, suffix string) resumeClaimSQLiteFixture {
	t.Helper()
	database, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := t.Context()
	createdAt := time.Unix(10, 0).UTC()
	interrupts := persistence.NewInterruptStore(sqlite.NewInterruptStore(database))
	checkpoints := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(database))
	transcriptStore := sqlite.NewTranscriptStore(database)
	runStore := sqlite.NewRunStore(database)
	pending := singleRunPending(
		t,
		"run_claim_"+suffix,
		"session_claim_"+suffix,
		"member_claim_"+suffix,
		"request_claim_"+suffix,
		"item_claim_"+suffix,
		createdAt,
		createdAt.Add(time.Second),
	)
	if err := interrupts.Open(ctx, pending); err != nil {
		t.Fatalf("open interrupt: %v", err)
	}
	questionItem, err := transcript.NewQuestion(transcript.ItemIdentity{
		SessionID:  pending.SessionID,
		RunID:      pending.Interrupts[0].RunID,
		ItemID:     pending.Interrupts[0].ItemID,
		OccurredAt: pending.Interrupts[0].ItemOccurredAt,
	}, *pending.Interrupts[0].Question)
	if err != nil {
		t.Fatalf("new question Item: %v", err)
	}
	if err := transcriptStore.AppendItem(ctx, questionItem); err != nil {
		t.Fatalf("seed question Item: %v", err)
	}
	root, found := pending.RootContinuation()
	if !found {
		t.Fatal("fixture Pending has no root continuation")
	}
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID:   root.MemberID,
		Payload:        []byte(`{"opaque":"tree"}`),
		BuildID:        checkpointBuildID,
		Scope:          runs.ExecutionScope{SessionID: pending.SessionID},
		ModelSelection: root.ModelSelection,
		Limits:         root.Limits,
	}
	if err := checkpoints.SaveCheckpoint(ctx, checkpoint); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}
	if err := runStore.Admit(ctx, run.Draft{
		RunID: pending.RootRunID, SessionID: pending.SessionID, SegmentID: "segment_claim_" + suffix,
		ModelSelection: root.ModelSelection, GoalIncarnationID: pending.GoalIncarnationID,
		Limits: root.Limits, Capabilities: pending.Capabilities, CreatedAt: root.RunCreatedAt,
	}); err != nil {
		t.Fatalf("admit claim root Run: %v", err)
	}
	runningRoot, found, err := runStore.Run(ctx, pending.RootRunID)
	if err != nil || !found {
		t.Fatalf("read claim root Run: found=%t err=%v", found, err)
	}
	waitingRoot, err := runningRoot.Suspend(pending.CreatedAt)
	if err != nil {
		t.Fatalf("park claim root Run: %v", err)
	}
	if err := runStore.Suspend(ctx, waitingRoot); err != nil {
		t.Fatalf("persist claim root park: %v", err)
	}
	answers := []runs.InterruptAnswer{{
		InterruptItemID: pending.Bindings[0].InterruptItemID,
		MemberID:        pending.Bindings[0].MemberID,
		RequestID:       pending.Bindings[0].RequestID,
		Resolution:      interrupt.Resolution{Answers: [][]string{{"continue"}}},
	}}
	return resumeClaimSQLiteFixture{
		ctx: ctx, db: database, pending: pending, answers: answers,
		claim: runs.ResumeClaimCommit{
			CommitID:  "run_commit_resume_claim_" + suffix,
			Expected:  pending,
			Answers:   answers,
			ClaimedAt: pending.CreatedAt.Add(time.Second),
		},
		checkpoint: checkpoint, question: questionItem, interrupts: interrupts,
		checkpoints: checkpoints, transcript: transcriptStore, runStore: runStore,
	}
}

func (fixture resumeClaimSQLiteFixture) effects(tx Transactor, state RunStore) *Effects {
	return New(Config{
		Interrupts:          fixture.interrupts,
		ResumeClaims:        fixture.interrupts,
		ExecutorCheckpoints: fixture.checkpoints,
		ItemReplacer:        fixture.transcript,
		State:               state,
		Tx:                  tx,
	})
}

func assertResumeClaimCommitted(t *testing.T, fixture resumeClaimSQLiteFixture) {
	t.Helper()
	if _, found, err := fixture.interrupts.Get(fixture.ctx, fixture.pending.RootRunID); err != nil || found {
		t.Fatalf("open interrupt after claim = found:%t err:%v, want hidden", found, err)
	}
	if _, err := fixture.checkpoints.LoadCheckpoint(fixture.ctx, fixture.checkpoint.RootMemberID); !errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("checkpoint after claim = %v, want not found", err)
	}
	answeredItem, found, err := fixture.transcript.Item(fixture.ctx, fixture.question.ID())
	if err != nil || !found {
		t.Fatalf("answered question Item = found:%t err:%v", found, err)
	}
	answeredQuestion, ok := answeredItem.Question()
	if !ok || !reflect.DeepEqual(answeredQuestion.Answers, [][]string{{"continue"}}) {
		t.Fatalf("answered question = %+v", answeredQuestion)
	}
}

func executorCheckpointValuesEqual(left, right runs.ExecutorCheckpoint) bool {
	return left.RootMemberID == right.RootMemberID &&
		slices.Equal(left.Payload, right.Payload) &&
		left.BuildID == right.BuildID &&
		left.Scope == right.Scope &&
		left.ModelSelection == right.ModelSelection &&
		left.Limits == right.Limits &&
		left.Capabilities.Equal(right.Capabilities) &&
		slices.Equal(left.Usage.Models, right.Usage.Models)
}

func TestCommitTerminalOwnsExecutorCheckpointDeletion(t *testing.T) {
	for _, test := range []struct {
		name                 string
		checkpointDeleteFail bool
		childCleanupFail     bool
	}{
		{name: "commit"},
		{name: "rollback when checkpoint delete fails", checkpointDeleteFail: true},
		{name: "rollback when child-start cleanup fails", childCleanupFail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newTerminalCheckpointFixture(t, test.checkpointDeleteFail, test.childCleanupFail)
			finished := finishedRunRecord("run_terminal", "ses_terminal", run.OutcomeCompleted)
			resolved := mustResolveMessageMark(t, *finished, 0)
			finished = &resolved
			err := fixture.effects.CommitEvent(fixture.ctx, runs.EventCommit{
				RunID: "run_terminal", SessionID: "ses_terminal", SegmentID: "seg_terminal", State: runs.StateTerminalize,
				CommitID: "event_commit_checkpoint",
				Outcome:  run.OutcomeCompleted, Run: finished,
				ObsoleteCheckpointRootID: fixture.rootMemberID,
			})
			if test.checkpointDeleteFail || test.childCleanupFail {
				assertTerminalCheckpointRollback(t, fixture, err)
				return
			}
			assertTerminalCheckpointCommit(t, fixture, err)
		})
	}
}

type terminalCheckpointFixture struct {
	ctx          context.Context
	database     *sql.DB
	runStore     *sqlite.RunStore
	checkpoints  *persistence.ExecutorCheckpointStore
	pending      runs.Pending
	rootMemberID string
	effects      *Effects
}

func newTerminalCheckpointFixture(
	t *testing.T,
	failCheckpointDeletion bool,
	failChildStartCleanup bool,
) terminalCheckpointFixture {
	t.Helper()
	database, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := t.Context()
	createdAt := time.Unix(1, 0).UTC()
	runStore := sqlite.NewRunStore(database)
	if err := runStore.Admit(ctx, run.Draft{
		RunID: "run_terminal", SessionID: "ses_terminal", SegmentID: "seg_terminal",
		CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(database))
	const rootMemberID = "member_terminal"
	if err := checkpointStore.SaveCheckpoint(ctx, executorCheckpoint(t, rootMemberID, "opaque terminal checkpoint", runs.ExecutorCheckpoint{
		BuildID: checkpointBuildID,
		Scope:   runs.ExecutionScope{SessionID: "ses_terminal"},
		Usage:   accounting.Snapshot{},
	})); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}
	interruptStore := persistence.NewInterruptStore(sqlite.NewInterruptStore(database))
	pending := singleRunPending(
		t,
		"run_terminal", "ses_terminal", rootMemberID, "request_terminal", "item_terminal",
		createdAt, createdAt.Add(time.Second),
	)
	if err := interruptStore.Open(ctx, pending); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}
	if _, found, err := interruptStore.ClaimResume(
		ctx,
		pending.SessionID,
		pending.RootRunID,
		[]runs.InterruptAnswer{{
			InterruptItemID: pending.Bindings[0].InterruptItemID,
			MemberID:        pending.Bindings[0].MemberID,
			RequestID:       pending.Bindings[0].RequestID,
			Resolution:      interrupt.Resolution{Answers: [][]string{{"continue"}}},
		}},
		createdAt.Add(2*time.Second),
	); err != nil || !found {
		t.Fatalf("seed answer claim: found=%t err=%v", found, err)
	}
	var checkpointDeleter ExecutorCheckpointStore = checkpointStore
	if failCheckpointDeletion {
		checkpointDeleter = failingExecutorCheckpointStore{
			ExecutorCheckpointStore: checkpointStore,
			err:                     errors.New("delete unavailable"),
		}
	}
	childStarts := ChildRunStartReservationStore(sqlite.NewChildRunStartReservationStore(database))
	if failChildStartCleanup {
		childStarts = failingChildRunStartReservationStore{
			ChildRunStartReservationStore: childStarts,
			err:                           errors.New("child-start cleanup unavailable"),
		}
	}
	effects := New(Config{
		State: runStore, Interrupts: interruptStore, ExecutorCheckpoints: checkpointDeleter,
		ChildRunStarts: childStarts,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, database, fn)
		},
	})
	return terminalCheckpointFixture{
		ctx: ctx, database: database, runStore: runStore, checkpoints: checkpointStore,
		pending: pending, rootMemberID: rootMemberID, effects: effects,
	}
}

type failingChildRunStartReservationStore struct {
	ChildRunStartReservationStore
	err error
}

func (store failingChildRunStartReservationStore) DeleteSession(context.Context, string) error {
	return store.err
}

func assertTerminalCheckpointRollback(t *testing.T, fixture terminalCheckpointFixture, commitError error) {
	t.Helper()
	if commitError == nil {
		t.Fatal("terminal commit succeeded after checkpoint deletion failed")
	}
	runsAfter, err := fixture.runStore.ListRuns(fixture.ctx, "ses_terminal")
	if err != nil || len(runsAfter) != 1 || runsAfter[0].State() != run.Running {
		t.Fatalf("Run after rollback = %+v, %v; want running", runsAfter, err)
	}
	var terminalCommitID string
	if err := fixture.database.QueryRowContext(
		fixture.ctx,
		`SELECT commit_id FROM runs WHERE run_id = ?`,
		"run_terminal",
	).Scan(&terminalCommitID); err != nil || terminalCommitID != "" {
		t.Fatalf("terminal marker after rollback = %q err=%v, want empty", terminalCommitID, err)
	}
	if _, err := fixture.checkpoints.LoadCheckpoint(fixture.ctx, fixture.rootMemberID); err != nil {
		t.Fatalf("checkpoint lost after terminal rollback: %v", err)
	}
	var state string
	if err := fixture.database.QueryRowContext(
		fixture.ctx,
		`SELECT state FROM interrupts WHERE root_run_id = ?`,
		fixture.pending.RootRunID,
	).Scan(&state); err != nil || state != "resuming" {
		t.Fatalf("answer claim after rollback = state:%q err:%v", state, err)
	}
}

func assertTerminalCheckpointCommit(t *testing.T, fixture terminalCheckpointFixture, commitError error) {
	t.Helper()
	if commitError != nil {
		t.Fatalf("CommitEvent: %v", commitError)
	}
	if _, err := fixture.checkpoints.LoadCheckpoint(fixture.ctx, fixture.rootMemberID); !errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("terminal checkpoint survived: %v", err)
	}
	var remaining int
	if err := fixture.database.QueryRowContext(
		fixture.ctx,
		`SELECT count(*) FROM interrupts WHERE root_run_id = ?`,
		fixture.pending.RootRunID,
	).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("terminal answer claim cleanup = rows:%d err:%v", remaining, err)
	}
}

type failingExecutorCheckpointStore struct {
	ExecutorCheckpointStore
	err error
}

func (store failingExecutorCheckpointStore) DeleteCheckpoints(context.Context, string, []string) error {
	return store.err
}

func TestCommitWaitingSubtreeCancellationCommitsCompleteWriteSet(t *testing.T) {
	fixture := newWaitingCancellationSQLiteFixture(t)
	result, err := fixture.effects.CommitWaitingSubtreeCancellation(
		fixture.ctx,
		fixture.commit,
	)
	if err != nil {
		t.Fatalf("CommitWaitingSubtreeCancellation: %v", err)
	}
	if result.TargetRun.ID() != fixture.childRun.ID() ||
		result.TargetRun.State() != run.Canceled ||
		result.RootRun.ID() != fixture.rootRun.ID() ||
		result.RootRun.State() != run.Running ||
		result.RootRun.ActiveSegmentID() != "segment_root_resumed" {
		t.Fatalf("result = %+v, want exact canceled child and resumed root", result)
	}
	if _, found, err := fixture.interrupts.Get(fixture.ctx, fixture.rootRun.ID()); err != nil || found {
		t.Fatalf("open Pending after commit found=%t err=%v, want consumed", found, err)
	}
	storedItem, found, err := fixture.transcript.Item(fixture.ctx, fixture.parentItem.ID())
	if err != nil || !found {
		t.Fatalf("parent Item after commit found=%t err=%v", found, err)
	}
	failure, failed := storedItem.Failure()
	if !failed || failure.Kind != tool.FailureChildRunCanceled {
		t.Fatalf("parent Item = %+v, want child_run_canceled", storedItem)
	}
	messages, err := fixture.conversation.Read(fixture.ctx, fixture.rootRun.SessionID())
	if err != nil || !reflect.DeepEqual(messages, fixture.commit.ConversationMessages) {
		t.Fatalf("conversation after child cancellation = %+v err=%v, want %+v", messages, err, fixture.commit.ConversationMessages)
	}
	for _, terminal := range fixture.commit.TerminalItems {
		item, found, err := fixture.transcript.Item(fixture.ctx, terminal.Expected.ID())
		if err != nil || !found || !sameItemSnapshot(item, terminal.Replacement) {
			t.Fatalf(
				"terminal interrupt Item %q = found:%t value:%+v err:%v, want %+v",
				terminal.Expected.ID(),
				found,
				item,
				err,
				terminal.Replacement,
			)
		}
	}
	assertStoredRunState(t, fixture.db, fixture.childRun.ID(), "terminal")
	assertStoredRunState(t, fixture.db, fixture.grandchildRun.ID(), "terminal")
	assertStoredRunState(t, fixture.db, fixture.rootRun.ID(), "running")
	checkpoint, err := fixture.checkpoints.LoadCheckpoint(fixture.ctx, "member_root")
	if err != nil {
		t.Fatalf("load replacement executor checkpoint: %v", err)
	}
	if !reflect.DeepEqual(
		normalizedExecutorCheckpoint(checkpoint),
		normalizedExecutorCheckpoint(fixture.replacementCheckpoint),
	) {
		t.Fatalf("replacement executor checkpoint = %+v, want %+v", checkpoint, fixture.replacementCheckpoint)
	}
}

func TestCommitWaitingSubtreeCancellationReconcilesAmbiguousCommit(t *testing.T) {
	tests := []struct {
		name              string
		survivingBoundary bool
		wantSegmentID     string
		wantRootState     run.State
	}{
		{
			name:              "remaining waiting boundary",
			survivingBoundary: true,
			wantRootState:     run.Waiting,
		},
		{
			name:          "resumed continuation",
			wantSegmentID: "segment_root_resumed",
			wantRootState: run.Running,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWaitingCancellationSQLiteFixtureWithSurvivingBoundary(
				t,
				test.survivingBoundary,
			)
			commitCtx, cancelCommit := context.WithCancel(fixture.ctx)
			t.Cleanup(cancelCommit)
			loseReceipt := true
			fixture.effects = sqliteEffects(
				sqliteOpeningStores{interrupts: fixture.interrupts, transcript: fixture.transcript},
				Config{
					ItemReplacer:        fixture.transcript,
					Conversation:        fixture.conversation,
					State:               fixture.runState,
					ExecutorCheckpoints: fixture.checkpoints,
					Tx: func(ctx context.Context, fn func(context.Context) error) error {
						err := sqlite.RunInTx(ctx, fixture.db, fn)
						if err == nil && loseReceipt {
							loseReceipt = false
							cancelCommit()
							return errors.New("lost waiting cancellation commit receipt")
						}
						return err
					},
				},
			)
			result, err := fixture.effects.CommitWaitingSubtreeCancellation(
				commitCtx,
				fixture.commit,
			)
			if err != nil {
				t.Fatalf("ambiguous waiting cancellation = %v, want reconciled success", err)
			}
			if result.RootRun.State() != test.wantRootState ||
				result.RootRun.ActiveSegmentID() != test.wantSegmentID {
				t.Fatalf(
					"reconciled root = state:%s segment:%q, want %s/%q",
					result.RootRun.State(),
					result.RootRun.ActiveSegmentID(),
					test.wantRootState,
					test.wantSegmentID,
				)
			}
			matched, err := fixture.runState.RunCommitCommitted(
				fixture.ctx,
				fixture.commit.SessionID,
				fixture.commit.RootRunID,
				test.wantSegmentID,
				fixture.commit.CommitID,
			)
			if err != nil || !matched {
				t.Fatalf("waiting cancellation marker matched=%t err=%v, want true/nil", matched, err)
			}
			replayed, err := fixture.effects.CommitWaitingSubtreeCancellation(
				commitCtx,
				fixture.commit,
			)
			if err != nil || !replayed.TargetRun.Equal(result.TargetRun) || !replayed.RootRun.Equal(result.RootRun) {
				t.Fatalf("exact replay result = %+v err=%v, want %+v", replayed, err, result)
			}
			messages, err := fixture.conversation.Read(fixture.ctx, fixture.commit.SessionID)
			if err != nil || len(messages) != len(fixture.commit.ConversationMessages) {
				t.Fatalf("conversation after exact replay = %d messages err=%v", len(messages), err)
			}
			other := fixture.commit
			other.CommitID = "run_commit_other_waiting_cancellation"
			if _, err := fixture.effects.CommitWaitingSubtreeCancellation(fixture.ctx, other); err == nil {
				t.Fatal("different waiting cancellation write-set reused the prior commit marker")
			}
			requireSQLiteHealthy(t, fixture.ctx, fixture.db)
		})
	}
}

func TestCommitWaitingSubtreeCancellationRollsBackCheckpointAndApplicationFacts(t *testing.T) {
	fixture := newWaitingCancellationSQLiteFixture(t)
	staleSnapshot := fixture.commit.ParentItem.Expected.Snapshot()
	staleSnapshot.Identity.OccurredAt = fixture.parentItem.OccurredAt().Add(-time.Second)
	stale, err := transcript.RestoreItem(staleSnapshot)
	if err != nil {
		t.Fatalf("restore stale parent Item: %v", err)
	}
	fixture.commit.ParentItem.Expected = stale

	if _, err := fixture.effects.CommitWaitingSubtreeCancellation(
		fixture.ctx,
		fixture.commit,
	); err == nil {
		t.Fatal("CommitWaitingSubtreeCancellation accepted a stale parent Item")
	}
	if _, found, err := fixture.interrupts.Get(fixture.ctx, fixture.rootRun.ID()); err != nil || !found {
		t.Fatalf("open Pending after rollback found=%t err=%v, want retained", found, err)
	}
	storedItem, found, err := fixture.transcript.Item(fixture.ctx, fixture.parentItem.ID())
	_, hasFailure := storedItem.Failure()
	if err != nil || !found || hasFailure {
		t.Fatalf("parent Item after rollback found=%t item=%+v err=%v", found, storedItem, err)
	}
	assertStoredRunState(t, fixture.db, fixture.childRun.ID(), "waiting")
	assertStoredRunState(t, fixture.db, fixture.grandchildRun.ID(), "waiting")
	assertStoredRunState(t, fixture.db, fixture.rootRun.ID(), "waiting")
	checkpoint, err := fixture.checkpoints.LoadCheckpoint(fixture.ctx, "member_root")
	if err != nil {
		t.Fatalf("load rolled-back executor checkpoint: %v", err)
	}
	if !reflect.DeepEqual(
		normalizedExecutorCheckpoint(checkpoint),
		normalizedExecutorCheckpoint(fixture.originalCheckpoint),
	) {
		t.Fatalf("rolled-back executor checkpoint = %+v, want %+v", checkpoint, fixture.originalCheckpoint)
	}
}

type waitingCancellationSQLiteFixture struct {
	ctx                   context.Context
	db                    *sql.DB
	effects               *Effects
	interrupts            *persistence.InterruptStore
	transcript            *sqlite.TranscriptStore
	conversation          *sqlite.MessageStore
	checkpoints           *persistence.ExecutorCheckpointStore
	runState              *sqlite.RunStore
	rootRun               run.Run
	childRun              run.Run
	grandchildRun         run.Run
	parentItem            transcript.Item
	originalItems         []transcript.Item
	originalCheckpoint    runs.ExecutorCheckpoint
	replacementCheckpoint runs.ExecutorCheckpoint
	commit                runs.WaitingSubtreeCancellationCommit
}

func newWaitingCancellationSQLiteFixture(t *testing.T) waitingCancellationSQLiteFixture {
	t.Helper()
	return newWaitingCancellationSQLiteFixtureWithSurvivingBoundary(t, false)
}

func newWaitingCancellationSQLiteFixtureWithSurvivingBoundary(
	t *testing.T,
	survivingBoundary bool,
) waitingCancellationSQLiteFixture {
	t.Helper()
	return newWaitingCancellationSQLiteFixtureAt(t, ":memory:", survivingBoundary)
}

func newWaitingCancellationSQLiteFixtureAt(
	t *testing.T,
	dataSourceName string,
	survivingBoundary bool,
) waitingCancellationSQLiteFixture {
	t.Helper()
	db, err := sqlite.Open(t.Context(), dataSourceName)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	createdAt := time.Date(2026, 7, 30, 1, 2, 3, 0, time.UTC)
	finishedAt := createdAt.Add(time.Minute)
	rootLineage := run.Lineage{}
	childLineage := run.Lineage{
		SpawnedByItemID: "item_spawn_child",
		ParentRunID:     "run_root",
		RootRunID:       "run_root",
	}
	grandchildLineage := run.Lineage{
		SpawnedByItemID: "item_spawn_grandchild",
		ParentRunID:     "run_child",
		RootRunID:       "run_root",
	}
	siblingLineage := run.Lineage{
		SpawnedByItemID: "item_spawn_sibling",
		ParentRunID:     "run_root",
		RootRunID:       "run_root",
	}
	state := sqlite.NewRunStore(db)
	capabilities := run.Capabilities{
		ChildRuns:      true,
		InterruptKinds: []interrupt.Kind{interrupt.Question},
	}
	if err := state.Admit(ctx, run.Draft{
		RunID: "run_root", SessionID: "session_1", SegmentID: "segment_root",
		Capabilities: capabilities,
		CreatedAt:    createdAt,
	}); err != nil {
		t.Fatalf("admit root: %v", err)
	}
	transcriptStore := sqlite.NewTranscriptStore(db)
	parentItem := itemfixture.MustRestore(itemfixture.Input{
		SessionID:  "session_1",
		ID:         childLineage.SpawnedByItemID,
		RunID:      childLineage.ParentRunID,
		Status:     transcript.ItemRunning,
		Kind:       transcript.ToolCall,
		OccurredAt: createdAt,
		Tool:       &transcript.ToolInvocation{Name: "delegate_task", Arguments: tool.Arguments{}},
	})
	if err := transcriptStore.AppendItem(ctx, parentItem); err != nil {
		t.Fatalf("seed spawning Item: %v", err)
	}
	if err := state.Admit(ctx, run.Draft{
		RunID:           "run_child",
		SessionID:       "session_1",
		SegmentID:       "segment_child",
		SpawnedByItemID: childLineage.SpawnedByItemID,
		ParentRunID:     childLineage.ParentRunID,
		RootRunID:       childLineage.RootRunID,
		CreatedAt:       createdAt,
	}); err != nil {
		t.Fatalf("admit child: %v", err)
	}
	if err := state.Admit(ctx, run.Draft{
		RunID:           "run_grandchild",
		SessionID:       "session_1",
		SegmentID:       "segment_grandchild",
		SpawnedByItemID: grandchildLineage.SpawnedByItemID,
		ParentRunID:     grandchildLineage.ParentRunID,
		RootRunID:       grandchildLineage.RootRunID,
		CreatedAt:       createdAt,
	}); err != nil {
		t.Fatalf("admit grandchild: %v", err)
	}
	if survivingBoundary {
		if err := state.Admit(ctx, run.Draft{
			RunID:           "run_sibling",
			SessionID:       "session_1",
			SegmentID:       "segment_sibling",
			SpawnedByItemID: siblingLineage.SpawnedByItemID,
			ParentRunID:     siblingLineage.ParentRunID,
			RootRunID:       siblingLineage.RootRunID,
			CreatedAt:       createdAt,
		}); err != nil {
			t.Fatalf("admit sibling: %v", err)
		}
	}
	grandchildQuestion := treeQuestion("item_grandchild_question", "run_grandchild")
	grandchildRun := waitingTestSessionRun(
		"run_grandchild",
		grandchildLineage,
		createdAt,
		[]transcript.Interrupt{grandchildQuestion},
	)
	grandchildRun = mutatedRun(grandchildRun, func(snapshot *run.Snapshot) {
		snapshot.Capabilities = capabilities
		snapshot.UpdatedAt = finishedAt
	})
	childQuestion := treeQuestion("item_child_question", "run_child")
	childRun := waitingTestSessionRun(
		"run_child",
		childLineage,
		createdAt,
		[]transcript.Interrupt{childQuestion},
	)
	childRun = mutatedRun(childRun, func(snapshot *run.Snapshot) {
		snapshot.Capabilities = capabilities
		snapshot.UpdatedAt = finishedAt
	})
	var siblingRun run.Run
	var siblingQuestion transcript.Interrupt
	if survivingBoundary {
		siblingQuestion = treeQuestion("item_sibling_question", "run_sibling")
		siblingRun = waitingTestSessionRun(
			"run_sibling",
			siblingLineage,
			createdAt,
			[]transcript.Interrupt{siblingQuestion},
		)
		siblingRun = mutatedRun(siblingRun, func(snapshot *run.Snapshot) {
			snapshot.Capabilities = capabilities
			snapshot.UpdatedAt = finishedAt
		})
	}
	rootRun := waitingTestSessionRun(
		"run_root",
		rootLineage,
		createdAt,
		nil,
	)
	rootRun = mutatedRun(rootRun, func(snapshot *run.Snapshot) {
		snapshot.Capabilities = capabilities
		snapshot.UpdatedAt = finishedAt
	})
	grandchildQuestionItem := itemfixture.MustRestore(itemfixture.Input{
		SessionID:  rootRun.SessionID(),
		ID:         grandchildQuestion.ItemID,
		RunID:      grandchildQuestion.RunID,
		Status:     transcript.ItemRunning,
		OccurredAt: createdAt,
		Kind:       transcript.QuestionItem,
		Question:   grandchildQuestion.Question,
	})
	childQuestionItem := itemfixture.MustRestore(itemfixture.Input{
		SessionID:  rootRun.SessionID(),
		ID:         childQuestion.ItemID,
		RunID:      childQuestion.RunID,
		Status:     transcript.ItemRunning,
		OccurredAt: createdAt,
		Kind:       transcript.QuestionItem,
		Question:   childQuestion.Question,
	})
	originalItems := []transcript.Item{
		parentItem,
		grandchildQuestionItem,
		childQuestionItem,
	}
	if survivingBoundary {
		originalItems = append(originalItems, itemfixture.MustRestore(itemfixture.Input{
			SessionID:  rootRun.SessionID(),
			ID:         siblingQuestion.ItemID,
			RunID:      siblingQuestion.RunID,
			Status:     transcript.ItemRunning,
			OccurredAt: createdAt,
			Kind:       transcript.QuestionItem,
			Question:   siblingQuestion.Question,
		}))
	}
	for _, item := range originalItems[1:] {
		if err := transcriptStore.AppendItem(ctx, item); err != nil {
			t.Fatalf("seed interrupt Item %q: %v", item.ID(), err)
		}
	}
	if err := state.Suspend(ctx, grandchildRun); err != nil {
		t.Fatalf("suspend grandchild: %v", err)
	}
	if err := state.Suspend(ctx, childRun); err != nil {
		t.Fatalf("suspend child: %v", err)
	}
	if survivingBoundary {
		if err := state.Suspend(ctx, siblingRun); err != nil {
			t.Fatalf("suspend sibling: %v", err)
		}
	}
	if err := state.Suspend(ctx, rootRun); err != nil {
		t.Fatalf("suspend root: %v", err)
	}

	pendingInterrupts := []transcript.Interrupt{grandchildQuestion, childQuestion}
	pendingBindings := []runs.InterruptBinding{
		{
			InterruptItemID: grandchildQuestion.ItemID,
			MemberID:        "member_grandchild",
			RequestID:       "request-member_grandchild",
		},
		{
			InterruptItemID: childQuestion.ItemID,
			MemberID:        "member_child",
			RequestID:       "request-member_child",
		},
	}
	pendingContinuations := []runs.Continuation{
		{
			RunID:        grandchildRun.ID(),
			MemberID:     "member_grandchild",
			Lineage:      grandchildLineage,
			RunCreatedAt: createdAt,
		},
		{
			RunID:        childRun.ID(),
			MemberID:     "member_child",
			Lineage:      childLineage,
			RunCreatedAt: createdAt,
		},
	}
	if survivingBoundary {
		pendingInterrupts = append(pendingInterrupts, siblingQuestion)
		pendingBindings = append(pendingBindings, runs.InterruptBinding{
			InterruptItemID: siblingQuestion.ItemID,
			MemberID:        "member_sibling",
			RequestID:       "request-member_sibling",
		})
		pendingContinuations = append(pendingContinuations, runs.Continuation{
			RunID:        siblingRun.ID(),
			MemberID:     "member_sibling",
			Lineage:      siblingLineage,
			RunCreatedAt: createdAt,
		})
	}
	pendingContinuations = append(pendingContinuations, runs.Continuation{
		RunID:        rootRun.ID(),
		MemberID:     "member_root",
		RunCreatedAt: createdAt,
		DrainedTools: []runs.DrainedTool{{
			ItemID: parentItem.ID(), ItemOccurredAt: parentItem.OccurredAt(),
			CallID: "call_child", SourceCallID: "provider_child",
			Name: "delegate_task", Arguments: "{}",
		}},
	})
	pending := runs.Pending{
		RootRunID:     rootRun.ID(),
		SessionID:     rootRun.SessionID(),
		ExecutorID:    "turn_1",
		Capabilities:  capabilities,
		Interrupts:    pendingInterrupts,
		Bindings:      pendingBindings,
		Continuations: pendingContinuations,
		CreatedAt:     finishedAt,
	}
	if err := pending.Validate(); err != nil {
		t.Fatalf("pending fixture: %v", err)
	}
	interruptStore := persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
	if err := interruptStore.Open(ctx, pending); err != nil {
		t.Fatalf("seed Pending: %v", err)
	}

	checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	const rootMemberID = "member_root"
	originalCheckpoint := executorCheckpoint(t, rootMemberID, "opaque checkpoint before cancellation", runs.ExecutorCheckpoint{
		BuildID: "original-build",
		Scope:   runs.ExecutionScope{SessionID: rootRun.SessionID()},
		Usage: accounting.Snapshot{Models: []accounting.ModelUsage{{
			Model:      "test-model",
			TokenUsage: accounting.TokenUsage{PromptTokens: 3, CompletionTokens: 2},
			CostUSD:    0.25,
			Calls:      1,
		}}},
	})
	if err := checkpointStore.SaveCheckpoint(ctx, originalCheckpoint); err != nil {
		t.Fatalf("seed executor checkpoint: %v", err)
	}
	terminalChild, err := childRun.CancelWaiting("stop delegated branch", finishedAt, 0)
	if err != nil {
		t.Fatalf("cancel child fixture: %v", err)
	}
	terminalGrandchild, err := grandchildRun.CancelWaiting("stop delegated branch", finishedAt, 0)
	if err != nil {
		t.Fatalf("cancel grandchild fixture: %v", err)
	}
	failure := tool.Failure{
		Kind:   tool.FailureChildRunCanceled,
		Detail: terminalChild.Detail(),
	}
	replacementItem, err := parentItem.AbandonToolCall(&failure, finishedAt)
	if err != nil {
		t.Fatalf("settle parent Item: %v", err)
	}
	var terminalItems []runs.ItemReplacement
	var (
		remainingPending *runs.Pending
		resume           *run.TreeResumeDraft
	)
	if survivingBoundary {
		reduced := pending
		reduced.Interrupts = slices.Clone(pending.Interrupts[2:])
		reduced.Bindings = slices.Clone(pending.Bindings[2:])
		rootContinuation := pending.Continuations[len(pending.Continuations)-1]
		rootContinuation.DrainedTools = nil
		rootContinuation.CommittedTools = []runs.CommittedTool{{
			ItemID:       parentItem.ID(),
			CallID:       "call_child",
			SourceCallID: "provider_child",
			Name:         "delegate_task",
			Arguments:    "{}",
			Failure:      failure,
		}}
		reduced.Continuations = []runs.Continuation{
			pending.Continuations[2],
			rootContinuation,
		}
		if err := reduced.Validate(); err != nil {
			t.Fatalf("reduced Pending fixture: %v", err)
		}
		remainingPending = &reduced
	} else {
		resume = &run.TreeResumeDraft{
			RootRunID: rootRun.ID(),
			SessionID: rootRun.SessionID(),
			ResumedAt: finishedAt,
			Runs: []run.ResumeDraft{{
				RunID: rootRun.ID(), SegmentID: "segment_root_resumed",
			}},
		}
	}
	replacementCheckpoint := executorCheckpoint(t, rootMemberID, "opaque checkpoint after cancellation", runs.ExecutorCheckpoint{
		BuildID: "original-build",
		Scope:   runs.ExecutionScope{SessionID: rootRun.SessionID()},
		Usage: accounting.Snapshot{Models: []accounting.ModelUsage{{
			Model:      "test-model",
			TokenUsage: accounting.TokenUsage{PromptTokens: 8, CompletionTokens: 5},
			CostUSD:    0.75,
			Calls:      2,
		}}},
	})
	conversationStore := sqlite.NewMessageStore(db)
	effects := sqliteEffects(
		sqliteOpeningStores{interrupts: interruptStore, transcript: transcriptStore},
		Config{
			ItemReplacer:        transcriptStore,
			Conversation:        conversationStore,
			State:               state,
			ExecutorCheckpoints: checkpointStore,
			Tx: func(ctx context.Context, fn func(context.Context) error) error {
				return sqlite.RunInTx(ctx, db, fn)
			},
		},
	)
	return waitingCancellationSQLiteFixture{
		ctx:                   ctx,
		db:                    db,
		effects:               effects,
		interrupts:            interruptStore,
		transcript:            transcriptStore,
		conversation:          conversationStore,
		checkpoints:           checkpointStore,
		runState:              state,
		rootRun:               rootRun,
		childRun:              childRun,
		grandchildRun:         grandchildRun,
		parentItem:            parentItem,
		originalItems:         originalItems,
		originalCheckpoint:    originalCheckpoint,
		replacementCheckpoint: replacementCheckpoint,
		commit: runs.WaitingSubtreeCancellationCommit{
			CommitID:         "run_commit_waiting_cancellation",
			RootRunID:        rootRun.ID(),
			TargetRunID:      childRun.ID(),
			SessionID:        rootRun.SessionID(),
			RootRun:          rootRun,
			ExpectedPending:  pending,
			RemainingPending: remainingPending,
			Checkpoint:       replacementCheckpoint,
			TerminalRuns:     []run.Run{terminalGrandchild, terminalChild},
			TerminalItems:    terminalItems,
			ParentItem: runs.ItemReplacement{
				Expected: parentItem, Replacement: replacementItem,
			},
			ConversationMessages: []chat.Message{chat.NewToolMessage(chat.ToolResult{
				ID: "provider_child", Name: "delegate_task",
				Result: "error: tool \"delegate_task\" failed: stop delegated branch", IsError: true,
			})},
			Resume: resume,
		},
	}
}

func executorCheckpoint(
	t *testing.T,
	rootMemberID string,
	payload string,
	checkpoint runs.ExecutorCheckpoint,
) runs.ExecutorCheckpoint {
	t.Helper()
	checkpoint.RootMemberID = rootMemberID
	checkpoint.Payload = []byte(payload)
	if err := checkpoint.Validate(); err != nil {
		t.Fatalf("executor checkpoint: %v", err)
	}
	return checkpoint
}

type sqliteOpeningStores struct {
	interrupts *persistence.InterruptStore
	transcript *sqlite.TranscriptStore
}

func sqliteEffects(stores sqliteOpeningStores, cfg Config) *Effects {
	cfg.Interrupts = stores.interrupts
	cfg.ResumeClaims = stores.interrupts
	cfg.Transcript = stores.transcript
	return New(cfg)
}

// TestCommitOpeningRefusesASecondOpenRun is the integration evidence that the
// opening transaction maintains session_has_at_most_one_open_run.
//
// The store's own Admit enforces the slot, but the invariant is about the whole
// opening write-set: an admission that loses the race must not leave the items it
// had already projected. Committing the projections and then failing the admit
// would produce a session whose history mentions a run that does not exist, and no
// later write supplies it.
func TestCommitOpeningRefusesASecondOpenRun(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	created := time.Now().UTC()
	history := sqlite.NewTranscriptStore(db)
	messages := sqlite.NewMessageStore(db)
	state := sqlite.NewRunStore(db)
	if err := state.Admit(ctx, run.Draft{RunID: "run_1", SessionID: "ses_1", SegmentID: "seg_open", CreatedAt: created}); err != nil {
		t.Fatalf("admit the first run: %v", err)
	}

	effects := sqliteEffects(sqliteOpeningStores{transcript: history}, Config{
		Conversation: messages,
		State:        state,
		Tx:           func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	second := run.Draft{RunID: "run_2", SessionID: "ses_1", SegmentID: "seg_open", CreatedAt: created}
	err = effects.CommitOpening(ctx, runs.OpeningCommit{
		CommitID: "run_commit_busy_opening", Admit: &second,
		Events: []runs.EventCommit{{
			RunID:     "run_2",
			SessionID: "ses_1",
			SegmentID: second.SegmentID,
			ConversationMessages: []chat.Message{
				chat.NewUserMessage(chat.NewTextPart("me too")),
			},
			Items: []transcript.Item{itemfixture.MustRestore(itemfixture.Input{
				SessionID: "ses_1", RunID: "run_2", ID: "item_second", OccurredAt: created,
				Status: transcript.ItemCompleted, Kind: transcript.UserMessage,
				Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "me too"}},
			})},
		}},
	})
	if !errors.Is(err, run.ErrSessionBusy) {
		t.Fatalf("CommitOpening against a busy session = %v, want ErrSessionBusy", err)
	}
	recorded, listErr := history.List(ctx, "ses_1")
	if listErr != nil || len(recorded) != 0 {
		t.Fatalf("history items=%d err=%v, want the refused opening to have written nothing", len(recorded), listErr)
	}
	if count, countErr := messages.Count(ctx, "ses_1"); countErr != nil || count != 0 {
		t.Fatalf("conversation messages=%d err=%v, want the refused opening to have written nothing", count, countErr)
	}
	var runRows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM runs WHERE session_id = ?`, "ses_1").Scan(&runRows); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runRows != 1 {
		t.Fatalf("runs rows = %d, want only the run that holds the slot", runRows)
	}
}

func TestCommitEventAppendsConversationBeforeResolvingTerminalWatermark(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	state := sqlite.NewRunStore(db)
	messages := sqlite.NewMessageStore(db)
	draft := run.Draft{
		RunID: "run_1", SessionID: "ses_1", SegmentID: "seg_open",
		CreatedAt: time.Unix(1, 0).UTC(),
	}
	if err := state.Admit(ctx, draft); err != nil {
		t.Fatalf("admit: %v", err)
	}
	effects := New(Config{
		Conversation: messages,
		State:        state,
		Tx:           func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	finished := finishedRunRecord(draft.RunID, draft.SessionID, run.OutcomeCompleted)
	if err := effects.CommitEvent(ctx, runs.EventCommit{
		RunID: draft.RunID, SessionID: draft.SessionID, SegmentID: draft.SegmentID,
		CommitID: "event_commit_watermark",
		State:    runs.StateTerminalize, Outcome: run.OutcomeCompleted, Run: finished,
		ConversationMessages: []chat.Message{
			chat.NewAssistantMessage(chat.NewTextPart("done")),
		},
	}); err != nil {
		t.Fatalf("CommitEvent: %v", err)
	}
	stored, err := messages.Read(ctx, draft.SessionID)
	if err != nil || len(stored) != 1 || stored[0].Text() != "done" {
		t.Fatalf("conversation = %#v, %v", stored, err)
	}
	runs, err := state.ListRuns(ctx, draft.SessionID)
	if err != nil || len(runs) != 1 || runs[0].MessageMark() != 1 {
		t.Fatalf("terminal Run = %#v, %v; want watermark 1", runs, err)
	}
}

// TestCommitEventReconcilesAmbiguousTerminalCommit proves that a terminal
// write-set whose COMMIT succeeded but whose success receipt was lost converges
// to success. The exact replay must do the same without appending its messages
// twice, while a different terminal result remains a conflict.
func TestCommitEventReconcilesAmbiguousTerminalCommit(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	state := sqlite.NewRunStore(db)
	messages := sqlite.NewMessageStore(db)
	draft := run.Draft{
		RunID: "run_ambiguous", SessionID: "ses_ambiguous", SegmentID: "seg_open",
		CreatedAt: time.Unix(1, 0).UTC(),
	}
	if err := state.Admit(ctx, draft); err != nil {
		t.Fatalf("admit: %v", err)
	}
	wantAmbiguous := errors.New("lost commit receipt")
	loseReceipt := true
	commitCtx, cancelCommit := context.WithCancel(ctx)
	t.Cleanup(cancelCommit)
	effects := New(Config{
		Conversation: messages,
		State:        state,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			err := sqlite.RunInTx(ctx, db, fn)
			if err == nil && loseReceipt {
				loseReceipt = false
				cancelCommit()
				return wantAmbiguous
			}
			return err
		},
	})
	finished := finishedRunRecord(draft.RunID, draft.SessionID, run.OutcomeCompleted)
	commit := runs.EventCommit{
		RunID: draft.RunID, SessionID: draft.SessionID, SegmentID: draft.SegmentID,
		CommitID: "event_commit_ambiguous",
		State:    runs.StateTerminalize, Outcome: run.OutcomeCompleted, Run: finished,
		ConversationMessages: []chat.Message{
			chat.NewAssistantMessage(chat.NewTextPart("durable answer")),
		},
	}
	if err := effects.CommitEvent(commitCtx, commit); err != nil {
		t.Fatalf("ambiguous CommitEvent = %v, want reconciled success", err)
	}
	stored, found, err := state.Run(ctx, draft.RunID)
	if err != nil || !found || stored.MessageMark() != 1 || !stored.State().IsTerminal() {
		t.Fatalf("terminal Run = %#v found=%t err=%v, want terminal watermark 1", stored, found, err)
	}
	var terminalSegmentID, terminalCommitID string
	if err := db.QueryRowContext(ctx,
		`SELECT commit_segment_id, commit_id FROM runs WHERE run_id = ?`,
		draft.RunID,
	).Scan(&terminalSegmentID, &terminalCommitID); err != nil {
		t.Fatalf("read terminal commit marker: %v", err)
	}
	if terminalSegmentID != draft.SegmentID || terminalCommitID != commit.CommitID {
		t.Fatalf(
			"terminal marker = %q/%q, want %q/%q",
			terminalSegmentID,
			terminalCommitID,
			draft.SegmentID,
			commit.CommitID,
		)
	}
	for _, test := range []struct {
		label     string
		segmentID string
		commitID  string
	}{
		{label: "other Segment", segmentID: "seg_other", commitID: commit.CommitID},
		{label: "other terminal attempt", segmentID: draft.SegmentID, commitID: "terminal_commit_other"},
	} {
		matched, matchErr := state.RunCommitCommitted(
			ctx,
			draft.SessionID,
			draft.RunID,
			test.segmentID,
			test.commitID,
		)
		if matchErr != nil || matched {
			t.Fatalf("%s marker matched=%t err=%v, want false/nil", test.label, matched, matchErr)
		}
	}
	assertSingleMessage := func(label string) {
		t.Helper()
		got, readErr := messages.Read(ctx, draft.SessionID)
		if readErr != nil || len(got) != 1 || got[0].Text() != "durable answer" {
			t.Fatalf("%s conversation = %#v err=%v, want one durable answer", label, got, readErr)
		}
	}
	assertSingleMessage("ambiguous commit")

	if err := effects.CommitEvent(commitCtx, commit); err != nil {
		t.Fatalf("exact terminal replay = %v, want idempotent success", err)
	}
	assertSingleMessage("exact replay")

	conflicting := mutatedRun(*finished, func(snapshot *run.Snapshot) {
		snapshot.Detail = "different terminal result"
	})
	commit.Run = &conflicting
	commit.CommitID = "event_commit_conflicting"
	if err := effects.CommitEvent(commitCtx, commit); err == nil {
		t.Fatal("different terminal replay succeeded")
	}

	otherDraft := run.Draft{
		RunID: "run_other", SessionID: "ses_other", SegmentID: "seg_other",
		CreatedAt: time.Unix(1, 0).UTC(),
	}
	if err := state.Admit(ctx, otherDraft); err != nil {
		t.Fatalf("admit other Run: %v", err)
	}
	otherFinished := finishedRunRecord(otherDraft.RunID, otherDraft.SessionID, run.OutcomeCompleted)
	if err := effects.CommitEvent(ctx, runs.EventCommit{
		RunID: otherDraft.RunID, SessionID: otherDraft.SessionID, SegmentID: otherDraft.SegmentID,
		CommitID: terminalCommitID,
		State:    runs.StateTerminalize, Outcome: run.OutcomeCompleted, Run: otherFinished,
	}); err == nil {
		t.Fatal("terminal commit identity was reused by another Run")
	}
	otherStored, found, err := state.Run(ctx, otherDraft.RunID)
	if err != nil || !found || otherStored.State() != run.Running || otherStored.ActiveSegmentID() != otherDraft.SegmentID {
		t.Fatalf("other Run after marker collision = %#v found=%t err=%v, want original running Segment", otherStored, found, err)
	}
	var integrity string
	if err := db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity_check = %q err=%v, want ok", integrity, err)
	}
	foreignKeys, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer foreignKeys.Close()
	if foreignKeys.Next() {
		t.Fatal("foreign_key_check reported a violation")
	}
	if err := foreignKeys.Err(); err != nil {
		t.Fatalf("foreign_key_check rows: %v", err)
	}
}

// TestCommitEventReconcilesAmbiguousAuthoritativeCommit proves that a
// non-terminal model/tool fact does not become an unknown external outcome
// merely because SQLite lost the success receipt after COMMIT. The Run pump is
// serial, so the exact latest marker remains available until this receipt has
// settled and is replaced only by the next canonical fact.
func TestCommitEventReconcilesAmbiguousAuthoritativeCommit(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	startedAt := time.Date(2026, 8, 15, 1, 2, 3, 0, time.UTC)
	finishedAt := startedAt.Add(time.Second)
	draft := run.Draft{
		RunID: "run_authoritative", SessionID: "ses_authoritative",
		SegmentID: "seg_authoritative", CreatedAt: startedAt,
	}
	state := sqlite.NewRunStore(db)
	if err := state.Admit(ctx, draft); err != nil {
		t.Fatalf("admit: %v", err)
	}
	messages := sqlite.NewMessageStore(db)
	invocations := sqlite.NewModelInvocationStore(db)
	baseConfig := Config{
		Conversation: messages, ModelInvocations: invocations,
		State: state, RunMetrics: state,
	}
	baseConfig.Tx = func(ctx context.Context, fn func(context.Context) error) error {
		return sqlite.RunInTx(ctx, db, fn)
	}
	effects := New(baseConfig)
	if err := effects.CommitEvent(ctx, runs.EventCommit{
		RunID: draft.RunID, SessionID: draft.SessionID, SegmentID: draft.SegmentID,
		CommitID: "event_commit_authoritative_start",
		ModelInvocations: []runs.ModelInvocationCommit{{
			CallID: "model_call_1", SegmentID: draft.SegmentID,
			State: runs.ModelInvocationStarted, StartedAt: startedAt,
		}},
	}); err != nil {
		t.Fatalf("commit model start: %v", err)
	}

	wantAmbiguous := errors.New("lost authoritative commit receipt")
	loseReceipt := true
	commitCtx, cancelCommit := context.WithCancel(ctx)
	t.Cleanup(cancelCommit)
	ambiguousConfig := baseConfig
	ambiguousConfig.Tx = func(ctx context.Context, fn func(context.Context) error) error {
		err := sqlite.RunInTx(ctx, db, fn)
		if err == nil && loseReceipt {
			loseReceipt = false
			cancelCommit()
			return wantAmbiguous
		}
		return err
	}
	ambiguousEffects := New(ambiguousConfig)
	usage := &accounting.Usage{Total: accounting.Totals{InputTokens: 2, OutputTokens: 1}}
	commit := runs.EventCommit{
		RunID: draft.RunID, SessionID: draft.SessionID, SegmentID: draft.SegmentID,
		CommitID: "event_commit_authoritative_complete",
		ConversationMessages: []chat.Message{
			chat.NewAssistantMessage(chat.NewTextPart("durable model answer")),
		},
		ModelInvocations: []runs.ModelInvocationCommit{{
			CallID: "model_call_1", SegmentID: draft.SegmentID,
			State: runs.ModelInvocationCompleted, StartedAt: startedAt, FinishedAt: finishedAt,
		}},
		Progress: &runs.RunProgressCommit{
			SegmentID: draft.SegmentID,
			Metrics: runfixture.MustMetrics(runfixture.MetricsInput{
				Steps: 1, Usage: usage,
			}),
			UpdatedAt: finishedAt,
		},
	}
	if err := ambiguousEffects.CommitEvent(commitCtx, commit); err != nil {
		t.Fatalf("ambiguous authoritative CommitEvent = %v, want reconciled success", err)
	}
	stored, found, err := state.Run(ctx, draft.RunID)
	if err != nil || !found || stored.State() != run.Running || stored.Metrics().Steps() != 1 {
		t.Fatalf("running Run = %#v found=%t err=%v, want one committed model step", stored, found, err)
	}
	var invocationState, eventSegmentID, eventCommitID string
	if err := db.QueryRowContext(ctx,
		`SELECT state FROM model_invocations WHERE call_id = ?`,
		"model_call_1",
	).Scan(&invocationState); err != nil || invocationState != "completed" {
		t.Fatalf("model invocation state = %q err=%v, want completed", invocationState, err)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT commit_segment_id, commit_id FROM runs WHERE run_id = ?`,
		draft.RunID,
	).Scan(&eventSegmentID, &eventCommitID); err != nil {
		t.Fatalf("read authoritative commit marker: %v", err)
	}
	if eventSegmentID != draft.SegmentID || eventCommitID != commit.CommitID {
		t.Fatalf(
			"authoritative marker = %q/%q, want %q/%q",
			eventSegmentID, eventCommitID, draft.SegmentID, commit.CommitID,
		)
	}
	assertSingleMessage := func(label string) {
		t.Helper()
		got, readErr := messages.Read(ctx, draft.SessionID)
		if readErr != nil || len(got) != 1 || got[0].Text() != "durable model answer" {
			t.Fatalf("%s conversation = %#v err=%v, want one durable answer", label, got, readErr)
		}
	}
	assertSingleMessage("ambiguous authoritative commit")

	// The original request context is canceled. The write cannot execute again,
	// but detached reconciliation still proves this exact transaction succeeded.
	if err := ambiguousEffects.CommitEvent(commitCtx, commit); err != nil {
		t.Fatalf("exact authoritative replay = %v, want idempotent success", err)
	}
	assertSingleMessage("exact authoritative replay")

	if err := effects.CommitEvent(ctx, runs.EventCommit{
		RunID: draft.RunID, SessionID: draft.SessionID, SegmentID: draft.SegmentID,
		CommitID: "event_commit_authoritative_later",
		ConversationMessages: []chat.Message{
			chat.NewAssistantMessage(chat.NewTextPart("later fact")),
		},
	}); err != nil {
		t.Fatalf("commit later fact: %v", err)
	}
	matched, err := state.RunCommitCommitted(
		ctx, draft.SessionID, draft.RunID, draft.SegmentID, commit.CommitID,
	)
	if err != nil || matched {
		t.Fatalf("superseded marker matched=%t err=%v, want false/nil", matched, err)
	}
	matched, err = state.RunCommitCommitted(
		ctx, draft.SessionID, draft.RunID, draft.SegmentID, "event_commit_authoritative_later",
	)
	if err != nil || !matched {
		t.Fatalf("latest marker matched=%t err=%v, want true/nil", matched, err)
	}
	var integrity string
	if err := db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity_check = %q err=%v, want ok", integrity, err)
	}
	foreignKeys, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer foreignKeys.Close()
	if foreignKeys.Next() {
		t.Fatal("foreign_key_check reported a violation")
	}
	if err := foreignKeys.Err(); err != nil {
		t.Fatalf("foreign_key_check rows: %v", err)
	}
}

// TestRootTerminalCommitReclaimsChildStartReservations pins the lifetime of
// the invisible child-start ledger to its active root tree. The rows retain a
// conclusive callback while the tree is live, but once the root terminal write
// commits no executor callback can legally replay. Keeping them forever leaks
// one durable row per delegated child and leaves reserve-before-crash rows
// unreachable.
func TestRootTerminalCommitReclaimsChildStartReservations(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	createdAt := time.Unix(1, 0).UTC()
	state := sqlite.NewRunStore(db)
	messages := sqlite.NewMessageStore(db)
	childStarts := sqlite.NewChildRunStartReservationStore(db)
	draft := run.Draft{
		RunID: "run_1", SessionID: "ses_1", SegmentID: "seg_open", CreatedAt: createdAt,
	}
	if err := state.Admit(ctx, draft); err != nil {
		t.Fatalf("admit: %v", err)
	}
	for _, record := range []sqlite.ChildRunStartReservationRecord{
		{MemberID: "member_child_1", SessionID: "ses_1", Payload: []byte(`{"run":"child_1"}`), CreatedAt: createdAt},
		{MemberID: "member_other", SessionID: "ses_2", Payload: []byte(`{"run":"other"}`), CreatedAt: createdAt},
	} {
		if err := childStarts.Reserve(ctx, record); err != nil {
			t.Fatalf("reserve child start %q: %v", record.MemberID, err)
		}
	}
	effects := New(Config{
		Interrupts:          persistence.NewInterruptStore(sqlite.NewInterruptStore(db)),
		Conversation:        messages,
		State:               state,
		ExecutorCheckpoints: persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db)),
		ChildRunStarts:      childStarts,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	})
	finished := finishedRunRecord(draft.RunID, draft.SessionID, run.OutcomeCompleted)
	if err := effects.CommitEvent(ctx, runs.EventCommit{
		RunID: draft.RunID, SessionID: draft.SessionID, SegmentID: draft.SegmentID,
		CommitID: "event_commit_cleanup",
		State:    runs.StateTerminalize, Outcome: run.OutcomeCompleted, Run: finished,
		ObsoleteCheckpointRootID: "member_root_1",
	}); err != nil {
		t.Fatalf("CommitEvent: %v", err)
	}
	var owned, foreign int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM child_run_start_reservations WHERE session_id = ?`, "ses_1",
	).Scan(&owned); err != nil {
		t.Fatalf("count terminal tree reservations: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM child_run_start_reservations WHERE session_id = ?`, "ses_2",
	).Scan(&foreign); err != nil {
		t.Fatalf("count foreign reservations: %v", err)
	}
	if owned != 0 || foreign != 1 {
		t.Fatalf("reservations after root terminal = owned:%d foreign:%d, want 0/1", owned, foreign)
	}
}

// TestCommitEventPersistsTheTerminalRunsResult is the integration evidence that
// the event transaction maintains terminal_run_explains_how_it_ended.
//
// The store refuses a terminal row without an outcome, but that is one table's
// rule. What the invariant is about is the durable projection a client later reads:
// a run that says it ended must come back with the facts explaining how, because no
// later write will supply them. Reading it back through ListRuns is the only way to
// see whether the commit carried them or merely flipped a state column.
func TestCommitEventPersistsTheTerminalRunsResult(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	history := sqlite.NewTranscriptStore(db)
	state := sqlite.NewRunStore(db)
	draft := run.Draft{RunID: "run_1", SessionID: "ses_1", SegmentID: "seg_open", CreatedAt: time.Unix(1, 0).UTC()}
	if err := state.Admit(ctx, draft); err != nil {
		t.Fatalf("admit: %v", err)
	}

	effects := sqliteEffects(sqliteOpeningStores{transcript: history}, Config{
		State: state,
		Tx:    func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	finished := finishedRunRecord(draft.RunID, draft.SessionID, run.OutcomeFailed)
	updated := mutatedRun(*finished, func(snapshot *run.Snapshot) {
		snapshot.Detail = "the provider rejected the request"
		snapshot.Metrics = runfixture.MustMetrics(runfixture.MetricsInput{Steps: 3})
		snapshot.Failure = &run.Failure{Kind: run.FailureProviderRejected, Detail: "rejected"}
	})
	updated = mustResolveMessageMark(t, updated, 0)
	finished = &updated
	if err := effects.CommitEvent(ctx, runs.EventCommit{
		RunID: draft.RunID, SessionID: draft.SessionID, SegmentID: draft.SegmentID, State: runs.StateTerminalize,
		CommitID: "event_commit_result",
		Outcome:  run.OutcomeFailed, Run: finished,
	}); err != nil {
		t.Fatalf("CommitEvent: %v", err)
	}

	recorded, err := state.ListRuns(ctx, draft.SessionID)
	if err != nil || len(recorded) != 1 {
		t.Fatalf("ListRuns = %d runs, %v", len(recorded), err)
	}
	record := recorded[0]
	switch {
	case record.State() != run.Failed:
		t.Errorf("run state = %v, want Failed", record.State())
	case !runHasOutcome(record, run.OutcomeFailed):
		t.Errorf("run outcome = %v, want the outcome it terminated with", record.Snapshot().Outcome)
	case record.Metrics().Steps() != 3:
		t.Errorf("run metrics = %+v, want the accrual the terminal commit carried", record.Metrics())
	case record.Detail() != "the provider rejected the request":
		t.Errorf("run detail = %q, want the explanation it ended with", record.Detail())
	case record.FinishedAt().IsZero():
		t.Error("terminal run has no finish time")
	}
}

// parkedRunRecord is the record a park hands to Suspend: the run moving to
// Waiting with the interrupt it is parked on.
func parkedRunRecord(runID, sessionID string, createdAt time.Time) run.Run {
	value, err := run.Admit(run.Draft{
		RunID: runID, SessionID: sessionID, SegmentID: "seg_open", CreatedAt: createdAt,
	})
	if err != nil {
		panic(err)
	}
	value, err = value.Suspend(createdAt)
	if err != nil {
		panic(err)
	}
	return value
}

func requireSQLiteHealthy(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	var integrity string
	if err := db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity_check = %q err=%v, want ok", integrity, err)
	}
	foreignKeys, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer foreignKeys.Close()
	if foreignKeys.Next() {
		t.Fatal("foreign_key_check reported a violation")
	}
	if err := foreignKeys.Err(); err != nil {
		t.Fatalf("foreign_key_check rows: %v", err)
	}
}
