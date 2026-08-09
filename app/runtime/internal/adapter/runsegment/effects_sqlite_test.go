package runsegment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

const checkpointBuildID = "test-build"

// executorTreeFixture is deliberately local and opaque to Runtime production
// code. Persistence tests need stable bytes and member identities, not an
// execution framework's private snapshot model.
type executorTreeFixture struct {
	RootID  string                  `json:"root_id"`
	Members []executorMemberFixture `json:"members"`
}

type executorMemberFixture struct {
	ID         string `json:"id"`
	ParentID   string `json:"parent_id,omitempty"`
	WaitPrompt string `json:"wait_prompt,omitempty"`
}

func waitingExecutorMember(id, prompt string) executorMemberFixture {
	return executorMemberFixture{ID: id, WaitPrompt: prompt}
}

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
	if err := state.Admit(ctx, run.Draft{RunID: "run_actual", SessionID: "ses_1", SegmentID: "seg_open", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := state.Suspend(ctx, parkedRunRecord("run_actual", "ses_1", time.Now().UTC())); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	stalePending := singleRunPending(
		t,
		"run_stale", "ses_1", "proc_stale", "susp_stale", "item_stale",
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
	err = effects.CommitOpening(ctx, runs.OpeningCommit{Resume: &resume, Events: []runs.EventCommit{{RunID: "run_stale", SessionID: "ses_1"}}})
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
		"run_1", "ses_1", "proc_1", "susp_1", "item_question",
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
		Resume: &resume,
		Events: []runs.EventCommit{{
			RunID:     "run_1",
			SessionID: "ses_1",
			Items: []transcript.Item{{
				SessionID: "ses_1", RunID: "run_1", ID: "item_resumed", OccurredAt: created,
				Status: transcript.ItemCompleted, Kind: transcript.UserMessage,
				Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "go on"}},
			}},
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

	if err := effects.CommitOpening(ctx, opening); err == nil {
		t.Fatal("duplicate CommitOpening succeeded after the Run was already resumed")
	}
	recorded, listErr = history.List(ctx, "ses_1")
	if listErr != nil || len(recorded) != 1 {
		t.Fatalf("history after duplicate items=%d err=%v, want unchanged", len(recorded), listErr)
	}
	if err := db.QueryRowContext(ctx, `SELECT state FROM runs WHERE run_id = ?`, "run_1").Scan(&stateName); err != nil || stateName != "running" {
		t.Fatalf("run state after duplicate=%q err=%v, want running", stateName, err)
	}
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
		RunID: "run_model", SessionID: "ses_model", ModelInvocations: []runs.ModelInvocationCommit{start},
	}); err != nil {
		t.Fatalf("commit start: %v", err)
	}

	item := transcript.Item{
		SessionID: "ses_model", RunID: "run_model", ID: "item_final",
		OccurredAt: finishedAt, Status: transcript.ItemCompleted, Kind: transcript.AgentMessage,
		Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "answer"}},
	}
	completion := runs.ModelInvocationCommit{
		CallID: "model_call_1", SegmentID: "seg_model", State: runs.ModelInvocationCompleted,
		StartedAt: startedAt, FinishedAt: finishedAt,
	}
	usage := &transcript.Usage{ModelUsage: transcript.ModelUsage{InputTokens: 2, OutputTokens: 1}}
	wrongSegment := runs.RunProgressCommit{
		SegmentID: "seg_wrong", Metrics: transcript.RunMetrics{Steps: 1, Usage: usage}, UpdatedAt: finishedAt,
	}
	err = effects.CommitEvent(ctx, runs.EventCommit{
		RunID: "run_model", SessionID: "ses_model", Items: []transcript.Item{item},
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
		RunID: "run_model", SessionID: "ses_model", Items: []transcript.Item{item},
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
	if err != nil || len(recorded) != 1 || recorded[0].ID != item.ID {
		t.Fatalf("history after commit = %#v err=%v", recorded, err)
	}
	persistedRun, found, err := runState.Run(ctx, "run_model")
	if err != nil || !found || persistedRun.Metrics.Steps != 1 || persistedRun.Metrics.Usage == nil ||
		persistedRun.Metrics.Usage.InputTokens != 2 || persistedRun.Metrics.Usage.OutputTokens != 1 {
		t.Fatalf("Run accounting after commit = %#v found=%v err=%v", persistedRun.Metrics, found, err)
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
		Transcript: history, ToolInvocations: invocations, RunMetrics: runState,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	})
	starts := []runs.ToolInvocationCommit{
		{CallID: "tool_first", ItemID: "item_first", SegmentID: "seg_tools", State: runs.ToolInvocationStarted, StartedAt: startedAt},
		{CallID: "tool_second", ItemID: "item_second", SegmentID: "seg_tools", State: runs.ToolInvocationStarted, StartedAt: startedAt},
	}
	for _, start := range starts {
		if err := effects.CommitEvent(ctx, runs.EventCommit{
			RunID: "run_tools", SessionID: "ses_tools", ToolInvocations: []runs.ToolInvocationCommit{start},
		}); err != nil {
			t.Fatalf("commit Tool start %q: %v", start.CallID, err)
		}
	}

	items := make([]transcript.Item, 0, 2)
	terminals := make([]runs.ToolInvocationCommit, 0, 2)
	for index, start := range starts {
		name := []string{"first", "second"}[index]
		result := tool.StringResult(name + "-result")
		items = append(items, transcript.Item{
			SessionID: "ses_tools", RunID: "run_tools", ID: start.ItemID,
			OccurredAt: startedAt, FinishedAt: finishedAt,
			Status: transcript.ItemCompleted, Kind: transcript.ToolCall,
			Tool:        &transcript.ToolInvocation{Name: name, Arguments: tool.Arguments{}, Result: &result},
			SafetyClass: tool.SafetyClassSafe,
		})
		terminals = append(terminals, runs.ToolInvocationCommit{
			CallID: start.CallID, ItemID: start.ItemID, SegmentID: start.SegmentID,
			State: runs.ToolInvocationCompleted, StartedAt: startedAt, FinishedAt: finishedAt,
		})
	}
	wrongSegment := runs.RunProgressCommit{
		SegmentID: "seg_wrong", Metrics: transcript.RunMetrics{}, UpdatedAt: finishedAt,
	}
	err = effects.CommitEvent(ctx, runs.EventCommit{
		RunID: "run_tools", SessionID: "ses_tools", Items: items,
		ToolInvocations: terminals, Progress: &wrongSegment,
	})
	if err == nil {
		t.Fatal("canonical Tool batch with a stale segment fence succeeded")
	}
	if recorded, err := history.List(ctx, "ses_tools"); err != nil || len(recorded) != 0 {
		t.Fatalf("history after rollback = %#v err=%v, want empty", recorded, err)
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
		RunID: "run_tools", SessionID: "ses_tools", Items: items,
		ToolInvocations: terminals, Progress: &progress,
	}); err != nil {
		t.Fatalf("commit canonical Tool batch: %v", err)
	}
	recorded, err := history.List(ctx, "ses_tools")
	if err != nil || len(recorded) != 2 || recorded[0].ID != "item_first" || recorded[1].ID != "item_second" {
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
			db, err := sqlite.Open(t.Context(), ":memory:")
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })
			ctx := t.Context()
			state := sqlite.NewRunStore(db)
			ints := persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
			createdAt := time.Now().UTC()
			if err := state.Admit(ctx, run.Draft{
				RunID: "run_root", SessionID: "session_1", SegmentID: "segment_root", CreatedAt: createdAt,
			}); err != nil {
				t.Fatalf("admit root: %v", err)
			}
			lineage := run.Lineage{
				SpawnedByItemID: "item_spawn_child",
				ParentRunID:     "run_root",
				RootRunID:       "run_root",
			}
			if err := state.Admit(ctx, run.Draft{
				RunID: "run_child", SessionID: "session_1", SegmentID: "segment_child",
				SpawnedByItemID: lineage.SpawnedByItemID,
				ParentRunID:     lineage.ParentRunID,
				RootRunID:       lineage.RootRunID,
				CreatedAt:       createdAt,
			}); err != nil {
				t.Fatalf("admit child: %v", err)
			}
			childRun := waitingTreeRun(
				"run_child",
				"session_1",
				lineage,
				createdAt,
				[]transcript.Interrupt{treeQuestion("item_child", "run_child")},
			)
			if err := state.Suspend(ctx, childRun); err != nil {
				t.Fatalf("suspend child: %v", err)
			}
			if test.suspendRoot {
				rootRun := waitingTreeRun(
					"run_root",
					"session_1",
					run.Lineage{},
					createdAt,
					nil,
				)
				if err := state.Suspend(ctx, rootRun); err != nil {
					t.Fatalf("suspend root: %v", err)
				}
			}
			pending := runs.Pending{
				RootRunID:  "run_root",
				SessionID:  "session_1",
				ExecutorID: "turn_1",
				Capabilities: run.Capabilities{
					ChildRuns:      true,
					InterruptKinds: []interrupt.Kind{interrupt.Question},
				},
				Interrupts: childRun.Interrupts,
				Bindings: []runs.InterruptBinding{{
					InterruptItemID: "item_child",
					MemberID:        "process_child",
					RequestID:       "suspension_child",
				}},
				Continuations: []runs.Continuation{
					{
						RunID:        "run_child",
						MemberID:     "process_child",
						Lineage:      lineage,
						RunCreatedAt: createdAt,
					},
					{
						RunID:        "run_root",
						MemberID:     "process_root",
						RunCreatedAt: createdAt,
					},
				},
				CreatedAt: createdAt.Add(time.Second),
			}
			if err := ints.Open(ctx, pending); err != nil {
				t.Fatalf("put pending: %v", err)
			}
			claimResumeForTest(t, ints, pending)
			effects := sqliteEffects(sqliteOpeningStores{interrupts: ints}, Config{
				State: state,
				Tx: func(ctx context.Context, fn func(context.Context) error) error {
					return sqlite.RunInTx(ctx, db, fn)
				},
			})
			resume := run.TreeResumeDraft{
				RootRunID: "run_root",
				SessionID: "session_1",
				ResumedAt: time.Now().UTC(),
				Runs: []run.ResumeDraft{
					{RunID: "run_child", SegmentID: "segment_child_resumed"},
					{RunID: "run_root", SegmentID: "segment_root_resumed"},
				},
			}
			err = effects.CommitOpening(ctx, runs.OpeningCommit{Resume: &resume})
			if test.wantError {
				if err == nil {
					t.Fatal("CommitOpening succeeded with a root Run that was not waiting")
				}
				assertStoredRunState(t, db, "run_child", "waiting")
				assertStoredRunState(t, db, "run_root", "running")
				if _, found, getErr := ints.Get(ctx, "run_root"); getErr != nil || found {
					t.Fatalf("pending after rollback found=%v err=%v, want hidden claim", found, getErr)
				}
				if err := ints.RequireResumeClaim(ctx, "session_1", "run_root"); err != nil {
					t.Fatalf("resume claim after rollback: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("CommitOpening: %v", err)
			}
			assertStoredRunState(t, db, "run_child", "running")
			assertStoredRunState(t, db, "run_root", "running")
			if _, found, getErr := ints.Get(ctx, "run_root"); getErr != nil || found {
				t.Fatalf("pending after commit found=%v err=%v, want hidden claim", found, getErr)
			}
			if err := ints.RequireResumeClaim(ctx, "session_1", "run_root"); err != nil {
				t.Fatalf("resume claim after commit: %v", err)
			}
		})
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

func waitingTreeRun(
	runID string,
	sessionID string,
	lineage run.Lineage,
	createdAt time.Time,
	open []transcript.Interrupt,
) transcript.Run {
	return transcript.Run{
		ID:              runID,
		SessionID:       sessionID,
		SpawnedByItemID: lineage.SpawnedByItemID,
		ParentRunID:     lineage.ParentRunID,
		RootRunID:       lineage.RootRunID,
		State:           run.Waiting,
		Interrupts:      open,
		CreatedAt:       createdAt,
		MessageMark:     transcript.UnknownMessageMark,
	}
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
	scheduled := session.Session{ID: draft.SessionID, Title: "scheduled", CWD: "/work", StartedAt: created, UpdatedAt: created, Revision: 1}
	err = effects.CommitOpening(ctx, runs.OpeningCommit{
		Admit:            &draft,
		ScheduledSession: &scheduled,
		// No firing is seeded: Accept fails after Ensure and Admit, so the
		// test exercises rollback rather than a preflight rejection.
		ScheduleFiring: "fire_missing",
		Events: []runs.EventCommit{{
			RunID: draft.RunID, SessionID: draft.SessionID,
			Run: &transcript.Run{ID: draft.RunID, SessionID: draft.SessionID, UpdatedAt: created},
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
	if err := sessions.Restore(ctx, session.Session{ID: "ses_goal", StartedAt: created, UpdatedAt: created, Revision: 1}); err != nil {
		t.Fatalf("seed goal session: %v", err)
	}
	selection := mustEffectSelection(t, "provider", "model")
	g, err := goal.New("ses_goal", "finish", selection, goal.Budget{MaxRuns: 1}, "lease_goal", created)
	if err != nil {
		t.Fatalf("new goal: %v", err)
	}
	if _, saved, err := goals.Save(ctx, g, goal.Version{}); err != nil || !saved {
		t.Fatalf("seed goal saved=%v err=%v", saved, err)
	}
	state := sqlite.NewRunStore(db)
	draft := run.Draft{
		RunID: "run_goal", SessionID: g.SessionID, SegmentID: "seg_open",
		GoalLeaseID: g.LeaseID, CreatedAt: created,
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
	// The watermark is already resolved, so this commit needs no message store.
	finished := finishedRunRecord(draft.RunID, draft.SessionID, run.OutcomeCompleted)
	finished.GoalLeaseID = g.LeaseID
	costUSD := 0.25
	finished.Metrics = transcript.RunMetrics{
		Steps: 2,
		Usage: &transcript.Usage{ModelUsage: transcript.ModelUsage{CostUSD: &costUSD}},
	}
	finished.MessageMark = 0
	if err := effects.CommitEvent(ctx, runs.EventCommit{
		RunID: draft.RunID, SessionID: draft.SessionID, State: runs.StateTerminalize,
		Outcome: run.OutcomeCompleted,
		Run:     finished,
		GoalRun: &goal.RunRecord{
			SessionID: g.SessionID, LeaseID: g.LeaseID, RunID: draft.RunID,
			Outcome: run.OutcomeCompleted, CostUSD: costUSD, Steps: 2, CompletedAt: finished.FinishedAt,
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
	snapshot := waitingExecutorMember("proc_1", "continue?")
	tree := executorTreeFixture{
		RootID:  snapshot.ID,
		Members: []executorMemberFixture{snapshot},
	}
	checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	checkpoint := executorCheckpoint(t, tree, runs.ExecutorCheckpoint{
		BuildID:        checkpointBuildID,
		Scope:          runs.ExecutionScope{SessionID: "ses_1"},
		ModelSelection: mustEffectSelection(t, "anthropic", "claude"),
		Usage:          accounting.Snapshot{},
	})
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}}
	open := []transcript.Interrupt{{
		ItemID:   "item_question",
		RunID:    "run_1",
		Kind:     interrupt.Question,
		Question: question,
	}}
	effects := sqliteEffects(sqliteOpeningStores{interrupts: ints, transcript: history}, Config{
		State:               state,
		ExecutorCheckpoints: checkpointStore,
		Tx:                  func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	pending := singleRunPending(
		t,
		"run_1", "ses_1", "proc_1", "request-proc_1", "item_question",
		createdAt, parkedAt,
	)
	if err := effects.CommitTreeBarrier(ctx, runs.TreeBarrierCommit{
		Pending:    pending,
		Checkpoint: checkpoint,
		Runs: []runs.EventCommit{{
			RunID: "run_1", SessionID: "ses_1", State: runs.StateSuspend,
			Items: []transcript.Item{{
				SessionID: "ses_1", ID: "item_question", RunID: "run_1",
				Status: transcript.ItemRunning, Kind: transcript.QuestionItem,
				Question: question, OccurredAt: parkedAt,
			}},
			Run: &transcript.Run{
				SessionID: "ses_1", ID: "run_1", State: run.Waiting,
				ModelSelection: pending.Continuations[0].ModelSelection,
				Capabilities:   pending.Capabilities,
				Interrupts:     open, CreatedAt: createdAt, UpdatedAt: parkedAt, MessageMark: -1,
			},
		}},
	}); err != nil {
		t.Fatalf("park: %v", err)
	}

	if stored, err := checkpointStore.LoadCheckpoint(ctx, snapshot.ID); err != nil || stored.RootMemberID != snapshot.ID {
		t.Fatalf("stored executor checkpoint = (%+v, %v)", stored, err)
	}
	if err := state.Admit(ctx, run.Draft{RunID: "run_next", SessionID: "ses_1", SegmentID: "seg_open", CreatedAt: parkedAt}); !errors.Is(err, run.ErrSessionBusy) {
		t.Fatalf("admit after intact park = %v, want ErrSessionBusy", err)
	}
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
	snapshot := waitingExecutorMember("proc_rollback", "continue?")
	checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	checkpoint := executorCheckpoint(t, executorTreeFixture{
		RootID: snapshot.ID, Members: []executorMemberFixture{snapshot},
	}, runs.ExecutorCheckpoint{
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
		"run_missing", "ses_rollback", snapshot.ID, "request-"+snapshot.ID, "item_question",
		createdAt, parkedAt,
	)
	parkedRun := parkedRunRecord("run_missing", "ses_rollback", createdAt)
	err = effects.CommitTreeBarrier(ctx, runs.TreeBarrierCommit{
		Pending:    pending,
		Checkpoint: checkpoint,
		Runs: []runs.EventCommit{{
			RunID: "run_missing", SessionID: "ses_rollback", State: runs.StateSuspend,
			Run: &parkedRun,
		}},
	})
	if err == nil {
		t.Fatal("CommitTreeBarrier succeeded without an admitted Run")
	}
	if _, loadErr := checkpointStore.LoadCheckpoint(ctx, snapshot.ID); !errors.Is(loadErr, runs.ErrExecutorCheckpointNotFound) {
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
	pending := singleRunPending(
		t,
		"run_claim", "session_claim", "member_claim", "request_claim", "item_claim",
		createdAt, createdAt.Add(time.Second),
	)
	if err := interruptStore.Open(ctx, pending); err != nil {
		t.Fatalf("open interrupt: %v", err)
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
	effects := New(Config{
		Interrupts:          interruptStore,
		ResumeClaims:        interruptStore,
		ExecutorCheckpoints: checkpointStore,
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

	stale := pending
	stale.ExecutorID = "turn_stale"
	if _, err := effects.ClaimResume(ctx, runs.ResumeClaimCommit{
		Expected: stale, Answers: answers, ClaimedAt: claimedAt,
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
		Expected: pending, Answers: answers, ClaimedAt: claimedAt,
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

func TestCommitTerminalOwnsExecutorCheckpointDeletion(t *testing.T) {
	for _, test := range []struct {
		name       string
		deleteFail bool
	}{
		{name: "commit"},
		{name: "rollback when checkpoint delete fails", deleteFail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, err := sqlite.Open(t.Context(), ":memory:")
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })
			ctx := t.Context()
			createdAt := time.Unix(1, 0).UTC()
			runStore := sqlite.NewRunStore(db)
			if err := runStore.Admit(ctx, run.Draft{
				RunID: "run_terminal", SessionID: "ses_terminal", SegmentID: "seg_terminal",
				CreatedAt: createdAt,
			}); err != nil {
				t.Fatalf("admit: %v", err)
			}
			checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
			snapshot := waitingExecutorMember("proc_terminal", "continue?")
			if err := checkpointStore.SaveCheckpoint(ctx, executorCheckpoint(t, executorTreeFixture{
				RootID: snapshot.ID, Members: []executorMemberFixture{snapshot},
			}, runs.ExecutorCheckpoint{
				BuildID: checkpointBuildID,
				Scope:   runs.ExecutionScope{SessionID: "ses_terminal"},
				Usage:   accounting.Snapshot{},
			})); err != nil {
				t.Fatalf("seed checkpoint: %v", err)
			}
			interruptStore := persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
			pending := singleRunPending(
				t,
				"run_terminal", "ses_terminal", snapshot.ID, "request_terminal", "item_terminal",
				createdAt, createdAt.Add(time.Second),
			)
			if err := interruptStore.Open(ctx, pending); err != nil {
				t.Fatalf("seed interrupt: %v", err)
			}
			if _, found, err := interruptStore.ClaimResume(ctx, pending.SessionID, pending.RootRunID, []runs.InterruptAnswer{{
				InterruptItemID: pending.Bindings[0].InterruptItemID,
				MemberID:        pending.Bindings[0].MemberID,
				RequestID:       pending.Bindings[0].RequestID,
				Resolution:      interrupt.Resolution{Answers: [][]string{{"continue"}}},
			}}, createdAt.Add(2*time.Second)); err != nil || !found {
				t.Fatalf("seed answer claim: found=%t err=%v", found, err)
			}

			var checkpoints ExecutorCheckpointStore = checkpointStore
			if test.deleteFail {
				checkpoints = failingExecutorCheckpointStore{
					ExecutorCheckpointStore: checkpointStore,
					err:                     errors.New("delete unavailable"),
				}
			}
			effects := New(Config{
				State:               runStore,
				Interrupts:          interruptStore,
				ExecutorCheckpoints: checkpoints,
				Tx: func(ctx context.Context, fn func(context.Context) error) error {
					return sqlite.RunInTx(ctx, db, fn)
				},
			})
			finished := finishedRunRecord("run_terminal", "ses_terminal", run.OutcomeCompleted)
			finished.MessageMark = 0
			err = effects.CommitEvent(ctx, runs.EventCommit{
				RunID: "run_terminal", SessionID: "ses_terminal", State: runs.StateTerminalize,
				Outcome: run.OutcomeCompleted, Run: finished, ObsoleteCheckpointRootID: snapshot.ID,
			})
			if test.deleteFail {
				if err == nil {
					t.Fatal("terminal commit succeeded after checkpoint deletion failed")
				}
				runsAfter, listErr := runStore.ListRuns(ctx, "ses_terminal")
				if listErr != nil || len(runsAfter) != 1 || runsAfter[0].State != run.Running {
					t.Fatalf("Run after rollback = %+v, %v; want running", runsAfter, listErr)
				}
				if _, loadErr := checkpointStore.LoadCheckpoint(ctx, snapshot.ID); loadErr != nil {
					t.Fatalf("checkpoint lost after terminal rollback: %v", loadErr)
				}
				var state string
				if scanErr := db.QueryRowContext(ctx,
					`SELECT state FROM interrupts WHERE root_run_id = ?`, pending.RootRunID,
				).Scan(&state); scanErr != nil || state != "resuming" {
					t.Fatalf("answer claim after rollback = state:%q err:%v", state, scanErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("CommitEvent: %v", err)
			}
			if _, loadErr := checkpointStore.LoadCheckpoint(ctx, snapshot.ID); !errors.Is(loadErr, runs.ErrExecutorCheckpointNotFound) {
				t.Fatalf("terminal checkpoint survived: %v", loadErr)
			}
			var remaining int
			if scanErr := db.QueryRowContext(ctx,
				`SELECT count(*) FROM interrupts WHERE root_run_id = ?`, pending.RootRunID,
			).Scan(&remaining); scanErr != nil || remaining != 0 {
				t.Fatalf("terminal answer claim cleanup = rows:%d err:%v", remaining, scanErr)
			}
		})
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
	if result.TargetRun.ID != fixture.childRun.ID ||
		result.TargetRun.State != run.Canceled ||
		result.RootRun.ID != fixture.rootRun.ID ||
		result.RootRun.State != run.Running ||
		result.RootRun.ActiveSegmentID != "segment_root_resumed" {
		t.Fatalf("result = %+v, want exact canceled child and resumed root", result)
	}
	if _, found, err := fixture.interrupts.Get(fixture.ctx, fixture.rootRun.ID); err != nil || found {
		t.Fatalf("open Pending after commit found=%t err=%v, want consumed", found, err)
	}
	storedItem, found, err := fixture.transcript.Item(fixture.ctx, fixture.parentItem.ID)
	if err != nil || !found {
		t.Fatalf("parent Item after commit found=%t err=%v", found, err)
	}
	if storedItem.Error == nil ||
		storedItem.Error.Kind != transcript.ChildRunCanceledProblem ||
		storedItem.Error.Scope != transcript.ToolProblem {
		t.Fatalf("parent Item = %+v, want child_run_canceled", storedItem)
	}
	for _, terminal := range fixture.commit.TerminalItems {
		item, found, err := fixture.transcript.Item(fixture.ctx, terminal.Expected.ID)
		if err != nil || !found || !sameItemSnapshot(item, terminal.Replacement) {
			t.Fatalf(
				"terminal interrupt Item %q = found:%t value:%+v err:%v, want %+v",
				terminal.Expected.ID,
				found,
				item,
				err,
				terminal.Replacement,
			)
		}
	}
	assertStoredRunState(t, fixture.db, fixture.childRun.ID, "terminal")
	assertStoredRunState(t, fixture.db, fixture.grandchildRun.ID, "terminal")
	assertStoredRunState(t, fixture.db, fixture.rootRun.ID, "running")
	checkpoint, err := fixture.checkpoints.LoadCheckpoint(fixture.ctx, "process_root")
	if err != nil {
		t.Fatalf("load replacement process tree: %v", err)
	}
	tree := restoredExecutorTree(t, checkpoint)
	if len(tree.Members) != 1 ||
		tree.Members[0].WaitPrompt != "after-cancel" ||
		checkpoint.BuildID != "original-build" {
		t.Fatalf("replacement process tree = %+v checkpoint=%+v", tree, checkpoint)
	}
}

func TestCommitWaitingSubtreeCancellationRollsBackCheckpointAndApplicationFacts(t *testing.T) {
	fixture := newWaitingCancellationSQLiteFixture(t)
	fixture.commit.ParentItem.Expected.OccurredAt = fixture.parentItem.OccurredAt.Add(time.Second)

	if _, err := fixture.effects.CommitWaitingSubtreeCancellation(
		fixture.ctx,
		fixture.commit,
	); err == nil {
		t.Fatal("CommitWaitingSubtreeCancellation accepted a stale parent Item")
	}
	if _, found, err := fixture.interrupts.Get(fixture.ctx, fixture.rootRun.ID); err != nil || !found {
		t.Fatalf("open Pending after rollback found=%t err=%v, want retained", found, err)
	}
	storedItem, found, err := fixture.transcript.Item(fixture.ctx, fixture.parentItem.ID)
	if err != nil || !found || storedItem.Error != nil {
		t.Fatalf("parent Item after rollback found=%t item=%+v err=%v", found, storedItem, err)
	}
	assertStoredRunState(t, fixture.db, fixture.childRun.ID, "waiting")
	assertStoredRunState(t, fixture.db, fixture.grandchildRun.ID, "waiting")
	assertStoredRunState(t, fixture.db, fixture.rootRun.ID, "waiting")
	checkpoint, err := fixture.checkpoints.LoadCheckpoint(fixture.ctx, "process_root")
	if err != nil {
		t.Fatalf("load rolled-back process tree: %v", err)
	}
	tree := restoredExecutorTree(t, checkpoint)
	rootSnapshot, found := executorMemberByID(tree, "process_root")
	if len(tree.Members) != 3 ||
		!found ||
		rootSnapshot.WaitPrompt != "before-cancel" ||
		checkpoint.BuildID != "original-build" {
		t.Fatalf("rolled-back process tree = %+v checkpoint=%+v", tree, checkpoint)
	}
}

type waitingCancellationSQLiteFixture struct {
	ctx                   context.Context
	db                    *sql.DB
	effects               *Effects
	interrupts            *persistence.InterruptStore
	transcript            *sqlite.TranscriptStore
	checkpoints           *persistence.ExecutorCheckpointStore
	runState              *sqlite.RunStore
	rootRun               transcript.Run
	childRun              transcript.Run
	grandchildRun         transcript.Run
	parentItem            transcript.Item
	originalItems         []transcript.Item
	originalTree          executorTreeFixture
	originalCheckpoint    runs.ExecutorCheckpoint
	replacementTree       executorTreeFixture
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
	parentItem := transcript.Item{
		SessionID:  "session_1",
		ID:         childLineage.SpawnedByItemID,
		RunID:      childLineage.ParentRunID,
		Status:     transcript.ItemIncomplete,
		Kind:       transcript.ToolCall,
		OccurredAt: createdAt,
		FinishedAt: createdAt,
		Tool:       &transcript.ToolInvocation{Name: "delegate_task", Arguments: tool.Arguments{}},
	}
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
	grandchildRun := waitingTreeRun(
		"run_grandchild",
		"session_1",
		grandchildLineage,
		createdAt,
		[]transcript.Interrupt{grandchildQuestion},
	)
	grandchildRun.Capabilities = capabilities
	grandchildRun.UpdatedAt = finishedAt
	childQuestion := treeQuestion("item_child_question", "run_child")
	childRun := waitingTreeRun(
		"run_child",
		"session_1",
		childLineage,
		createdAt,
		[]transcript.Interrupt{childQuestion},
	)
	childRun.Capabilities = capabilities
	childRun.UpdatedAt = finishedAt
	var siblingRun transcript.Run
	if survivingBoundary {
		siblingQuestion := treeQuestion("item_sibling_question", "run_sibling")
		siblingRun = waitingTreeRun(
			"run_sibling",
			"session_1",
			siblingLineage,
			createdAt,
			[]transcript.Interrupt{siblingQuestion},
		)
		siblingRun.Capabilities = capabilities
		siblingRun.UpdatedAt = finishedAt
	}
	rootRun := waitingTreeRun(
		"run_root",
		"session_1",
		rootLineage,
		createdAt,
		nil,
	)
	rootRun.Capabilities = capabilities
	rootRun.UpdatedAt = finishedAt
	grandchildQuestionItem := transcript.Item{
		SessionID:  rootRun.SessionID,
		ID:         grandchildQuestion.ItemID,
		RunID:      grandchildQuestion.RunID,
		Status:     transcript.ItemRunning,
		OccurredAt: createdAt,
		Kind:       transcript.QuestionItem,
		Question:   grandchildQuestion.Question,
	}
	childQuestionItem := transcript.Item{
		SessionID:  rootRun.SessionID,
		ID:         childQuestion.ItemID,
		RunID:      childQuestion.RunID,
		Status:     transcript.ItemRunning,
		OccurredAt: createdAt,
		Kind:       transcript.QuestionItem,
		Question:   childQuestion.Question,
	}
	originalItems := []transcript.Item{
		parentItem,
		grandchildQuestionItem,
		childQuestionItem,
	}
	if survivingBoundary {
		siblingQuestion := siblingRun.Interrupts[0]
		originalItems = append(originalItems, transcript.Item{
			SessionID:  rootRun.SessionID,
			ID:         siblingQuestion.ItemID,
			RunID:      siblingQuestion.RunID,
			Status:     transcript.ItemRunning,
			OccurredAt: createdAt,
			Kind:       transcript.QuestionItem,
			Question:   siblingQuestion.Question,
		})
	}
	for _, item := range originalItems[1:] {
		if err := transcriptStore.AppendItem(ctx, item); err != nil {
			t.Fatalf("seed interrupt Item %q: %v", item.ID, err)
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
	pendingSuspensions := []runs.InterruptBinding{
		{
			InterruptItemID: grandchildQuestion.ItemID,
			MemberID:        "process_grandchild",
			RequestID:       "request-process_grandchild",
		},
		{
			InterruptItemID: childQuestion.ItemID,
			MemberID:        "process_child",
			RequestID:       "request-process_child",
		},
	}
	pendingContinuations := []runs.Continuation{
		{
			RunID:        grandchildRun.ID,
			MemberID:     "process_grandchild",
			Lineage:      grandchildLineage,
			RunCreatedAt: createdAt,
		},
		{
			RunID:        childRun.ID,
			MemberID:     "process_child",
			Lineage:      childLineage,
			RunCreatedAt: createdAt,
		},
	}
	if survivingBoundary {
		siblingQuestion := siblingRun.Interrupts[0]
		pendingInterrupts = append(pendingInterrupts, siblingQuestion)
		pendingSuspensions = append(pendingSuspensions, runs.InterruptBinding{
			InterruptItemID: siblingQuestion.ItemID,
			MemberID:        "process_sibling",
			RequestID:       "request-process_sibling",
		})
		pendingContinuations = append(pendingContinuations, runs.Continuation{
			RunID:        siblingRun.ID,
			MemberID:     "process_sibling",
			Lineage:      siblingLineage,
			RunCreatedAt: createdAt,
		})
	}
	pendingContinuations = append(pendingContinuations, runs.Continuation{
		RunID:        rootRun.ID,
		MemberID:     "process_root",
		RunCreatedAt: createdAt,
		DrainedTools: []runs.DrainedTool{{
			ItemID: parentItem.ID, ItemOccurredAt: parentItem.OccurredAt,
			CallID: "call_child", Name: "delegate_task", Arguments: "{}",
		}},
	})
	pending := runs.Pending{
		RootRunID:     rootRun.ID,
		SessionID:     rootRun.SessionID,
		ExecutorID:    "turn_1",
		Capabilities:  capabilities,
		Interrupts:    pendingInterrupts,
		Bindings:      pendingSuspensions,
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
	originalRoot := waitingExecutorMember("process_root", "before-cancel")
	originalChild := waitingExecutorMember("process_child", "continue?")
	originalChild.ParentID = originalRoot.ID
	originalGrandchild := waitingExecutorMember("process_grandchild", "continue?")
	originalGrandchild.ParentID = originalChild.ID
	originalMembers := []executorMemberFixture{
		originalRoot,
		originalChild,
		originalGrandchild,
	}
	var originalSibling executorMemberFixture
	if survivingBoundary {
		originalSibling = waitingExecutorMember("process_sibling", "continue?")
		originalSibling.ParentID = originalRoot.ID
		originalMembers = append(originalMembers, originalSibling)
	}
	originalTree := executorTreeFixture{
		RootID:  originalRoot.ID,
		Members: originalMembers,
	}
	originalCheckpoint := executorCheckpoint(t, originalTree, runs.ExecutorCheckpoint{
		BuildID: "original-build",
		Scope:   runs.ExecutionScope{SessionID: rootRun.SessionID},
		Usage: accounting.Snapshot{Models: []accounting.ModelUsage{{
			Model:      "test-model",
			TokenUsage: accounting.TokenUsage{PromptTokens: 3, CompletionTokens: 2},
			CostUSD:    0.25,
			Calls:      1,
		}}},
	})
	if err := checkpointStore.SaveCheckpoint(ctx, originalCheckpoint); err != nil {
		t.Fatalf("seed process tree: %v", err)
	}
	replacementRoot := originalRoot
	replacementRoot.WaitPrompt = "after-cancel"
	replacementMembers := []executorMemberFixture{replacementRoot}
	if survivingBoundary {
		replacementMembers = append(replacementMembers, originalSibling)
	}
	replacementTree := executorTreeFixture{
		RootID:  replacementRoot.ID,
		Members: replacementMembers,
	}

	outcome := run.OutcomeCanceled
	terminalChild := childRun
	terminalChild.State = run.Canceled
	terminalChild.Outcome = &outcome
	terminalChild.Detail = "stop delegated branch"
	terminalChild.Interrupts = nil
	terminalChild.FinishedAt = finishedAt
	terminalChild.UpdatedAt = finishedAt
	terminalChild.MessageMark = 0
	terminalGrandchild := grandchildRun
	terminalGrandchild.State = run.Canceled
	terminalGrandchild.Outcome = &outcome
	terminalGrandchild.Detail = terminalChild.Detail
	terminalGrandchild.Interrupts = nil
	terminalGrandchild.FinishedAt = finishedAt
	terminalGrandchild.UpdatedAt = finishedAt
	terminalGrandchild.MessageMark = 0
	problem := transcript.Problem{
		Kind:   transcript.ChildRunCanceledProblem,
		Scope:  transcript.ToolProblem,
		Detail: terminalChild.Detail,
	}
	replacementItem := parentItem
	replacementItem.Error = &problem
	terminalItems := make([]runs.ItemReplacement, 0, 2)
	for _, item := range []transcript.Item{grandchildQuestionItem, childQuestionItem} {
		replacement := item
		replacement.Status = transcript.ItemIncomplete
		terminalItems = append(terminalItems, runs.ItemReplacement{
			Expected:    item,
			Replacement: replacement,
		})
	}
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
			ItemID:    parentItem.ID,
			CallID:    "call_child",
			Name:      "delegate_task",
			Arguments: "{}",
			Problem:   problem,
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
			RootRunID: rootRun.ID,
			SessionID: rootRun.SessionID,
			ResumedAt: finishedAt,
			Runs: []run.ResumeDraft{{
				RunID: rootRun.ID, SegmentID: "segment_root_resumed",
			}},
		}
	}
	replacementCheckpoint := executorCheckpoint(t, replacementTree, runs.ExecutorCheckpoint{
		BuildID: "original-build",
		Scope:   runs.ExecutionScope{SessionID: rootRun.SessionID},
		Usage: accounting.Snapshot{Models: []accounting.ModelUsage{{
			Model:      "test-model",
			TokenUsage: accounting.TokenUsage{PromptTokens: 8, CompletionTokens: 5},
			CostUSD:    0.75,
			Calls:      2,
		}}},
	})
	effects := sqliteEffects(
		sqliteOpeningStores{interrupts: interruptStore, transcript: transcriptStore},
		Config{
			ItemReplacer:        transcriptStore,
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
		checkpoints:           checkpointStore,
		runState:              state,
		rootRun:               rootRun,
		childRun:              childRun,
		grandchildRun:         grandchildRun,
		parentItem:            parentItem,
		originalItems:         originalItems,
		originalTree:          originalTree,
		originalCheckpoint:    originalCheckpoint,
		replacementTree:       replacementTree,
		replacementCheckpoint: replacementCheckpoint,
		commit: runs.WaitingSubtreeCancellationCommit{
			RootRunID:        rootRun.ID,
			TargetRunID:      childRun.ID,
			SessionID:        rootRun.SessionID,
			RootRun:          rootRun,
			ExpectedPending:  pending,
			RemainingPending: remainingPending,
			Checkpoint:       replacementCheckpoint,
			TerminalRuns:     []transcript.Run{terminalGrandchild, terminalChild},
			TerminalItems:    terminalItems,
			ParentItem: runs.ItemReplacement{
				Expected: parentItem, Replacement: replacementItem,
			},
			Resume: resume,
		},
	}
}

func executorMemberByID(
	tree executorTreeFixture,
	memberID string,
) (executorMemberFixture, bool) {
	for _, member := range tree.Members {
		if member.ID == memberID {
			return member, true
		}
	}
	return executorMemberFixture{}, false
}

func executorCheckpoint(
	t *testing.T,
	tree executorTreeFixture,
	checkpoint runs.ExecutorCheckpoint,
) runs.ExecutorCheckpoint {
	t.Helper()
	payload, err := json.Marshal(tree)
	if err != nil {
		t.Fatalf("encode executor process tree %q: %v", tree.RootID, err)
	}
	checkpoint.RootMemberID = tree.RootID
	checkpoint.Payload = payload
	if err := checkpoint.Validate(); err != nil {
		t.Fatalf("executor checkpoint: %v", err)
	}
	return checkpoint
}

func restoredExecutorTree(t *testing.T, checkpoint runs.ExecutorCheckpoint) executorTreeFixture {
	t.Helper()
	var tree executorTreeFixture
	if err := json.Unmarshal(checkpoint.Payload, &tree); err != nil {
		t.Fatalf("decode executor tree fixture %q: %v", checkpoint.RootMemberID, err)
	}
	if tree.RootID == "" || len(tree.Members) == 0 {
		t.Fatalf("decode executor tree fixture: invalid tree %+v", tree)
	}
	seen := make(map[string]struct{}, len(tree.Members))
	for _, member := range tree.Members {
		if member.ID == "" {
			t.Fatalf("decode executor tree fixture: empty member in %+v", tree)
		}
		if _, duplicate := seen[member.ID]; duplicate {
			t.Fatalf("decode executor tree fixture: duplicate member %q", member.ID)
		}
		seen[member.ID] = struct{}{}
	}
	if _, found := seen[tree.RootID]; !found {
		t.Fatalf("decode executor tree fixture: root %q is absent", tree.RootID)
	}
	return tree
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
	state := sqlite.NewRunStore(db)
	if err := state.Admit(ctx, run.Draft{RunID: "run_1", SessionID: "ses_1", SegmentID: "seg_open", CreatedAt: created}); err != nil {
		t.Fatalf("admit the first run: %v", err)
	}

	effects := sqliteEffects(sqliteOpeningStores{transcript: history}, Config{
		State: state,
		Tx:    func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	second := run.Draft{RunID: "run_2", SessionID: "ses_1", SegmentID: "seg_open", CreatedAt: created}
	err = effects.CommitOpening(ctx, runs.OpeningCommit{
		Admit: &second,
		Events: []runs.EventCommit{{
			RunID:     "run_2",
			SessionID: "ses_1",
			Items: []transcript.Item{{
				SessionID: "ses_1", RunID: "run_2", ID: "item_second", OccurredAt: created,
				Status: transcript.ItemCompleted, Kind: transcript.UserMessage,
				Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "me too"}},
			}},
		}},
	})
	if !errors.Is(err, run.ErrSessionBusy) {
		t.Fatalf("CommitOpening against a busy session = %v, want ErrSessionBusy", err)
	}
	recorded, listErr := history.List(ctx, "ses_1")
	if listErr != nil || len(recorded) != 0 {
		t.Fatalf("history items=%d err=%v, want the refused opening to have written nothing", len(recorded), listErr)
	}
	var runRows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM runs WHERE session_id = ?`, "ses_1").Scan(&runRows); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runRows != 1 {
		t.Fatalf("runs rows = %d, want only the run that holds the slot", runRows)
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
	finished.MessageMark = 0
	finished.Detail = "the provider rejected the request"
	finished.Metrics = transcript.RunMetrics{Steps: 3}
	finished.Error = &transcript.Problem{Kind: transcript.ProviderRejectedProblem, Detail: "rejected"}
	if err := effects.CommitEvent(ctx, runs.EventCommit{
		RunID: draft.RunID, SessionID: draft.SessionID, State: runs.StateTerminalize,
		Outcome: run.OutcomeFailed, Run: finished,
	}); err != nil {
		t.Fatalf("CommitEvent: %v", err)
	}

	recorded, err := state.ListRuns(ctx, draft.SessionID)
	if err != nil || len(recorded) != 1 {
		t.Fatalf("ListRuns = %d runs, %v", len(recorded), err)
	}
	record := recorded[0]
	switch {
	case record.State != run.Failed:
		t.Errorf("run state = %v, want Failed", record.State)
	case record.Outcome == nil || *record.Outcome != run.OutcomeFailed:
		t.Errorf("run outcome = %v, want the outcome it terminated with", record.Outcome)
	case record.Metrics.Steps != 3:
		t.Errorf("run metrics = %+v, want the accrual the terminal commit carried", record.Metrics)
	case record.Detail != "the provider rejected the request":
		t.Errorf("run detail = %q, want the explanation it ended with", record.Detail)
	case record.FinishedAt.IsZero():
		t.Error("terminal run has no finish time")
	}
}

// parkedRunRecord is the record a park hands to Suspend: the run moving to
// Waiting with the interrupt it is parked on.
func parkedRunRecord(runID, sessionID string, createdAt time.Time) transcript.Run {
	return transcript.Run{
		SessionID: sessionID, ID: runID, State: run.Waiting,
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_" + runID, Kind: interrupt.Question,
			Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "continue?"}}},
		}},
		CreatedAt: createdAt, MessageMark: transcript.UnknownMessageMark,
	}
}
