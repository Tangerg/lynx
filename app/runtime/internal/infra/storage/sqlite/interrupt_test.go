package sqlite_test

import (
	"context"
	"path/filepath"
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

func TestInterruptStore_PutGetListDelete(t *testing.T) {
	ctx := context.Background()
	store := newInterruptStore(t)

	p := interrupts.Pending{
		RootRunID: "run_1",
		SessionID: "ses_a",
		TurnID:    "turn_1",
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_question", RunID: "run_1", Kind: execution.QuestionInterrupt,
			Question: &transcript.Question{Prompt: "Choose"},
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
	if err := store.Put(ctx, p); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// UPSERT overwrite.
	p.SessionID = "ses_b"
	if err := store.Put(ctx, p); err != nil {
		t.Fatalf("Put overwrite: %v", err)
	}

	got, ok, err := store.Get(ctx, "run_1")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if got.SessionID != "ses_b" || len(got.Interrupts) != 1 || got.Interrupts[0].ItemID != "item_question" || !got.CreatedAt.Equal(time.Unix(5, 0).UTC()) {
		t.Fatalf("Get returned %+v", got)
	}
	// Per-run model selection round-trips (T1.4 — cross-restart rehydrate rebuilds
	// the SAME model client instead of dropping to the default).
	root, _ := got.RootContinuation()
	if root.ModelSelection.Provider() != "anthropic" || root.ModelSelection.Model() != "claude-opus-4-8" {
		t.Fatalf("Get provider/model = %q/%q, want anthropic/claude-opus-4-8", root.ModelSelection.Provider(), root.ModelSelection.Model())
	}

	if list, _ := store.List(ctx, "ses_b"); len(list) != 1 {
		t.Fatalf("List(ses_b) = %d, want 1", len(list))
	}
	if list, _ := store.List(ctx, ""); len(list) != 1 {
		t.Fatalf("List(all) = %d, want 1", len(list))
	}
	if list, _ := store.List(ctx, "nope"); len(list) != 0 {
		t.Fatalf("List(nope) = %d, want 0", len(list))
	}

	if err := store.Delete(ctx, "run_1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete(ctx, "run_1"); err != nil {
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
	if _, ok, err := store.Consume(ctx, "run_x"); err != nil || ok {
		t.Fatalf("Consume(empty) = ok=%v err=%v, want ok=false", ok, err)
	}

	if err := store.Put(ctx, interrupts.Pending{
		RootRunID: "run_1",
		SessionID: "ses_a",
		TurnID:    "turn_1",
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
		t.Fatalf("Put: %v", err)
	}

	// First consume returns the full record.
	got, ok, err := store.Consume(ctx, "run_1")
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
	if _, ok, err := store.Consume(ctx, "run_1"); err != nil || ok {
		t.Fatalf("second Consume = ok=%v err=%v, want ok=false — record must be gone", ok, err)
	}
}

func TestInterruptStore_RoundTripsContinuationTopology(t *testing.T) {
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
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_child", RunID: "run_child", Kind: execution.QuestionInterrupt,
			Question: &transcript.Question{Prompt: "Continue?"},
		}},
		Suspensions: []interrupts.SuspensionBinding{{
			InterruptItemID: "item_child",
			ProcessID:       "process_child",
			SuspensionID:    "suspension_child",
		}},
		Continuations: []interrupts.Continuation{
			{
				RunID:           "run_child",
				ProcessID:       "process_child",
				ParentProcessID: "process_root",
				SpawnCallID:     "spawn_child",
				Lineage:         lineage,
				RunCreatedAt:    createdAt,
			},
			{
				RunID:        "run_root",
				ProcessID:    "process_root",
				RunCreatedAt: createdAt,
				CommittedTools: []interrupts.CommittedTool{{
					ItemID: "item_spawn_child", CallID: "call_child", Name: "task", Arguments: "{}",
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
	if err := store.Put(t.Context(), pending); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, found, err := store.Get(t.Context(), pending.RootRunID)
	if err != nil || !found {
		t.Fatalf("Get: found=%v err=%v", found, err)
	}
	child := got.Continuations[0]
	if child.Lineage != lineage ||
		child.ParentProcessID != "process_root" ||
		child.SpawnCallID != "spawn_child" {
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

func TestInterruptStore_ProcessSnapshotHasOneOwner(t *testing.T) {
	store := newInterruptStore(t)
	ctx := t.Context()
	for _, runID := range []string{"run_1", "run_2"} {
		err := store.Put(ctx, interrupts.Pending{
			RootRunID: runID, SessionID: "ses_" + runID, TurnID: "turn_" + runID,
			Interrupts: []transcript.Interrupt{{
				ItemID: "item_" + runID, RunID: runID,
				Kind: execution.QuestionInterrupt, Question: &transcript.Question{Prompt: "continue?"},
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
			t.Fatalf("first Put: %v", err)
		}
		if runID == "run_2" && err == nil {
			t.Fatal("second Put reused a process snapshot owned by another interrupt")
		}
	}
}
