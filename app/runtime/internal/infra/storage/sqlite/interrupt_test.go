package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func newInterruptStore(t *testing.T) *sqlite.InterruptStore {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewInterruptStore(db)
}

func TestInterruptStore_OpenGetListDelete(t *testing.T) {
	ctx := context.Background()
	store := newInterruptStore(t)

	p := interrupts.Pending{
		RootRunID:   "run_1",
		SessionID:   "ses_a",
		TurnID:      "turn_1",
		GoalLeaseID: "goal-lease-1",
		ProtocolProfile: execution.RunProtocolProfile{
			InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
		},
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_question", RunID: "run_1", Kind: execution.QuestionInterrupt,
			Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Choose"}}},
		}},
		Suspensions: []interrupts.SuspensionBinding{{
			InterruptItemID: "item_question",
			ProcessID:       "proc_1",
			SuspensionID:    "suspension_1",
		}},
		Continuations: []interrupts.Continuation{{
			RunID:          "run_1",
			ProcessID:      "proc_1",
			ModelSelection: testModelSelection(t, "anthropic", "claude-opus-4-8"),
			RunCreatedAt:   time.Unix(1, 0).UTC(),
		}},
		CreatedAt: time.Unix(5, 0).UTC(),
	}
	if err := store.Open(ctx, p); err != nil {
		t.Fatalf("Open: %v", err)
	}
	// A second barrier cannot overwrite the one already waiting for a decision.
	p.SessionID = "ses_b"
	if err := store.Open(ctx, p); !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("Open duplicate error = %v, want ErrIdentityConflict", err)
	}

	got, ok, err := store.Get(ctx, "run_1")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if got.SessionID != "ses_a" || got.GoalLeaseID != p.GoalLeaseID || len(got.Interrupts) != 1 || got.Interrupts[0].ItemID != "item_question" || !got.CreatedAt.Equal(time.Unix(5, 0).UTC()) {
		t.Fatalf("Get returned %+v", got)
	}
	// Per-run model selection round-trips (T1.4 — cross-restart rehydrate rebuilds
	// the SAME model client instead of dropping to the default).
	root, _ := got.RootContinuation()
	if root.ModelSelection.Provider() != "anthropic" || root.ModelSelection.Model() != "claude-opus-4-8" {
		t.Fatalf("Get provider/model = %q/%q, want anthropic/claude-opus-4-8", root.ModelSelection.Provider(), root.ModelSelection.Model())
	}

	if list, _ := store.List(ctx, "ses_a"); len(list) != 1 {
		t.Fatalf("List(ses_a) = %d, want 1", len(list))
	}
	if list, _ := store.List(ctx, ""); len(list) != 1 {
		t.Fatalf("List(all) = %d, want 1", len(list))
	}
	if list, _ := store.List(ctx, "nope"); len(list) != 0 {
		t.Fatalf("List(nope) = %d, want 0", len(list))
	}

	if err := store.Delete(ctx, "ses_a", "run_1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete(ctx, "ses_a", "run_1"); err != nil {
		t.Fatalf("Delete not idempotent: %v", err)
	}
	if _, ok, _ := store.Get(ctx, "run_1"); ok {
		t.Fatal("Get after Delete: still present")
	}
}

// TestInterruptStore_ConsumeIsAtomic pins the resume-idempotency fix: Consume
// reads AND deletes the pending interrupt in one statement, so two concurrent
// resumes can't both claim it (the second gets ok=false and backs off, instead
// of both rebuilding the parked process and re-firing the approved tool). Also
// exercises that modernc SQLite supports DELETE ... RETURNING.
func TestInterruptStore_ConsumeIsAtomic(t *testing.T) {
	ctx := context.Background()
	store := newInterruptStore(t)

	// Nothing recorded → ok=false.
	if _, ok, err := store.Consume(ctx, "ses_a", "run_x"); err != nil || ok {
		t.Fatalf("Consume(empty) = ok=%v err=%v, want ok=false", ok, err)
	}

	if err := store.Open(ctx, interrupts.Pending{
		RootRunID: "run_1",
		SessionID: "ses_a",
		TurnID:    "turn_1",
		ProtocolProfile: execution.RunProtocolProfile{
			InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
		},
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_approval", RunID: "run_1", Kind: execution.ApprovalInterrupt,
			Approval: &transcript.Approval{Risk: tool.RiskHigh},
		}},
		Suspensions: []interrupts.SuspensionBinding{{
			InterruptItemID: "item_approval",
			ProcessID:       "proc_1",
			SuspensionID:    "suspension_1",
		}},
		Continuations: []interrupts.Continuation{{
			RunID: "run_1", ProcessID: "proc_1", RunCreatedAt: time.Unix(1, 0).UTC(),
		}},
		CreatedAt: time.Unix(7, 0).UTC(),
	}); err != nil {
		t.Fatalf("Open: %v", err)
	}

	// First consume returns the full record.
	got, ok, err := store.Consume(ctx, "ses_a", "run_1")
	if err != nil || !ok {
		t.Fatalf("Consume: ok=%v err=%v", ok, err)
	}
	root, _ := got.RootContinuation()
	if root.ProcessID != "proc_1" || len(got.Interrupts) != 1 || got.Interrupts[0].ItemID != "item_approval" ||
		got.Interrupts[0].Approval == nil || got.Interrupts[0].Approval.Risk != tool.RiskHigh {
		t.Fatalf("Consume returned %+v", got)
	}

	// Second consume finds nothing — the record was removed atomically with
	// the read, so a racing resume can't re-fire the tool.
	if _, ok, err := store.Consume(ctx, "ses_a", "run_1"); err != nil || ok {
		t.Fatalf("second Consume = ok=%v err=%v, want ok=false — record must be gone", ok, err)
	}
}

func TestInterruptStoreRejectsForeignSessionMutation(t *testing.T) {
	store := newInterruptStore(t)
	pending := interrupts.Pending{
		RootRunID: "run_1", SessionID: "ses_a", TurnID: "turn_1",
		ProtocolProfile: execution.RunProtocolProfile{
			InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
		},
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_question", RunID: "run_1", Kind: execution.QuestionInterrupt,
			Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}},
		}},
		Suspensions: []interrupts.SuspensionBinding{{
			InterruptItemID: "item_question", ProcessID: "process_root", SuspensionID: "suspension_root",
		}},
		Continuations: []interrupts.Continuation{{
			RunID: "run_1", ProcessID: "process_root", RunCreatedAt: time.Unix(1, 0).UTC(),
		}},
		CreatedAt: time.Unix(2, 0).UTC(),
	}
	if err := store.Open(t.Context(), pending); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, ok, err := store.Consume(t.Context(), "ses_b", pending.RootRunID); ok || !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("foreign Consume = ok:%t err:%v, want identity conflict", ok, err)
	}
	if err := store.Delete(t.Context(), "ses_b", pending.RootRunID); !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("foreign Delete error = %v, want identity conflict", err)
	}
	if stored, found, err := store.Get(t.Context(), pending.RootRunID); err != nil || !found || stored.SessionID != pending.SessionID {
		t.Fatalf("Pending after foreign mutations = found:%t value:%+v err:%v", found, stored, err)
	}
}

func TestInterruptStoreRoundTripsAppLineageWithoutExecutorTopology(t *testing.T) {
	store := newInterruptStore(t)
	createdAt := time.Unix(10, 0).UTC()
	lineage := execution.RunLineage{
		SpawnedByItemID: "item_spawn_child",
		ParentRunID:     "run_root",
		RootRunID:       "run_root",
	}
	pending := interrupts.Pending{
		RootRunID: "run_root",
		SessionID: "session_1",
		TurnID:    "turn_1",
		ProtocolProfile: execution.RunProtocolProfile{
			ChildRuns: true, InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
		},
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_child", RunID: "run_child", Kind: execution.QuestionInterrupt,
			Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}},
		}},
		Suspensions: []interrupts.SuspensionBinding{{
			InterruptItemID: "item_child",
			ProcessID:       "process_child",
			SuspensionID:    "suspension_child",
		}},
		Continuations: []interrupts.Continuation{
			{
				RunID:        "run_child",
				ProcessID:    "process_child",
				Lineage:      lineage,
				RunCreatedAt: createdAt,
			},
			{
				RunID:        "run_root",
				ProcessID:    "process_root",
				RunCreatedAt: createdAt,
				CommittedTools: []interrupts.CommittedTool{{
					ItemID: "item_spawn_child", CallID: "call_child", Name: "delegate_task", Arguments: "{}",
					Problem: transcript.Problem{
						Kind:   transcript.ChildRunCanceledProblem,
						Scope:  transcript.ToolProblem,
						Detail: "stop delegated branch",
					},
				}},
			},
		},
		CreatedAt: createdAt.Add(time.Second),
	}
	if err := store.Open(t.Context(), pending); err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, found, err := store.Get(t.Context(), pending.RootRunID)
	if err != nil || !found {
		t.Fatalf("Get: found=%v err=%v", found, err)
	}
	child := got.Continuations[0]
	if child.Lineage != lineage || child.ProcessID != "process_child" {
		t.Fatalf("child continuation = %+v, want lineage %+v", child, lineage)
	}
	root, found := got.RootContinuation()
	if !found ||
		len(root.CommittedTools) != 1 ||
		root.CommittedTools[0].ItemID != "item_spawn_child" ||
		root.CommittedTools[0].CallID != "call_child" ||
		root.CommittedTools[0].Problem.Kind != transcript.ChildRunCanceledProblem {
		t.Fatalf("root committed tools = %+v, want canceled child result hand-off", root.CommittedTools)
	}
}

func TestInterruptStoreRejectsUnknownExecutorTopologyFields(t *testing.T) {
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	store := sqlite.NewInterruptStore(database)
	pending := interrupts.Pending{
		RootRunID: "run_root", SessionID: "session_1", TurnID: "turn_1",
		ProtocolProfile: execution.RunProtocolProfile{
			InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
		},
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_question", RunID: "run_root", Kind: execution.QuestionInterrupt,
			Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}},
		}},
		Suspensions: []interrupts.SuspensionBinding{{
			InterruptItemID: "item_question", ProcessID: "process_root", SuspensionID: "suspension_root",
		}},
		Continuations: []interrupts.Continuation{{
			RunID: "run_root", ProcessID: "process_root", RunCreatedAt: time.Unix(1, 0).UTC(),
		}},
		CreatedAt: time.Unix(2, 0).UTC(),
	}
	if err := store.Open(t.Context(), pending); err != nil {
		t.Fatalf("Open interrupt: %v", err)
	}
	var encoded string
	if err := database.QueryRowContext(
		t.Context(),
		`SELECT continuations FROM interrupts WHERE root_run_id = ?`,
		pending.RootRunID,
	).Scan(&encoded); err != nil {
		t.Fatalf("read continuation JSON: %v", err)
	}
	poisoned := strings.Replace(
		encoded,
		`"processId":"process_root"`,
		`"processId":"process_root","parentProcessId":"legacy_parent"`,
		1,
	)
	if poisoned == encoded {
		t.Fatalf("continuation JSON does not contain process identity: %s", encoded)
	}
	if _, err := database.ExecContext(
		t.Context(),
		`UPDATE interrupts SET continuations = ? WHERE root_run_id = ?`,
		poisoned,
		pending.RootRunID,
	); err != nil {
		t.Fatalf("inject unknown topology field: %v", err)
	}
	if _, _, err := store.Get(t.Context(), pending.RootRunID); err == nil || !strings.Contains(err.Error(), `unknown field "parentProcessId"`) {
		t.Fatalf("Get error = %v, want unknown executor topology field", err)
	}
}

func TestInterruptStoreExecutorRootHasOnePendingOwner(t *testing.T) {
	store := newInterruptStore(t)
	ctx := t.Context()
	for _, runID := range []string{"run_1", "run_2"} {
		err := store.Open(ctx, interrupts.Pending{
			RootRunID: runID, SessionID: "ses_" + runID, TurnID: "turn_" + runID,
			ProtocolProfile: execution.RunProtocolProfile{
				InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
			},
			Interrupts: []transcript.Interrupt{{
				ItemID: "item_" + runID, RunID: runID,
				Kind: execution.QuestionInterrupt, Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "continue?"}}},
			}},
			Suspensions: []interrupts.SuspensionBinding{{
				InterruptItemID: "item_" + runID,
				ProcessID:       "proc_shared",
				SuspensionID:    "suspension_" + runID,
			}},
			Continuations: []interrupts.Continuation{{
				RunID: runID, ProcessID: "proc_shared", RunCreatedAt: time.Unix(1, 0).UTC(),
			}},
			CreatedAt: time.Unix(2, 0).UTC(),
		})
		if runID == "run_1" && err != nil {
			t.Fatalf("first Open: %v", err)
		}
		if runID == "run_2" && err == nil {
			t.Fatal("second Open reused an executor checkpoint root owned by another Pending")
		}
	}
}
