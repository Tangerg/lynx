package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
)

func newInterruptStore(t *testing.T) *persistence.InterruptStore {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
}

func TestInterruptStore_OpenGetListDelete(t *testing.T) {
	ctx := context.Background()
	store := newInterruptStore(t)

	p := runs.Pending{
		RootRunID:         "run_1",
		SessionID:         "ses_a",
		ExecutorID:        "turn_1",
		GoalIncarnationID: "goal-lease-1",
		Capabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_question", ItemOccurredAt: time.Unix(2, 0).UTC(), RunID: "run_1", Kind: interrupt.Question,
			Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Choose"}}},
		}},
		Bindings: []runs.InterruptBinding{{
			InterruptItemID: "item_question",
			MemberID:        "member_1",
			RequestID:       "request_1",
		}},
		Continuations: []runs.Continuation{{
			RunID:          "run_1",
			MemberID:       "member_1",
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
	if got.SessionID != "ses_a" || got.GoalIncarnationID != p.GoalIncarnationID || len(got.Interrupts) != 1 ||
		got.Interrupts[0].ItemID != "item_question" || !got.Interrupts[0].ItemOccurredAt.Equal(time.Unix(2, 0).UTC()) ||
		!got.CreatedAt.Equal(time.Unix(5, 0).UTC()) {
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
// of both rebuilding the parked executor tree and re-firing the approved tool). Also
// exercises that modernc SQLite supports DELETE ... RETURNING.
func TestInterruptStore_ConsumeIsAtomic(t *testing.T) {
	ctx := context.Background()
	store := newInterruptStore(t)
	arguments, err := tool.ParseArguments(`{"command":"go test ./..."}`)
	if err != nil {
		t.Fatalf("parse approval arguments: %v", err)
	}

	// Nothing recorded → ok=false.
	if _, ok, err := store.Consume(ctx, "ses_a", "run_x"); err != nil || ok {
		t.Fatalf("Consume(empty) = ok=%v err=%v, want ok=false", ok, err)
	}

	if err := store.Open(ctx, runs.Pending{
		RootRunID:  "run_1",
		SessionID:  "ses_a",
		ExecutorID: "turn_1",
		Capabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Approval},
		},
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_approval", ItemOccurredAt: time.Unix(2, 0).UTC(), RunID: "run_1", Kind: interrupt.Approval,
			Approval: &transcript.Approval{
				Tool: transcript.ToolInvocation{Name: "shell", Arguments: arguments},
				Risk: tool.RiskHigh, Reason: "executes tests", Rememberable: true,
			},
		}},
		Bindings: []runs.InterruptBinding{{
			InterruptItemID: "item_approval",
			MemberID:        "member_1",
			RequestID:       "request_1",
			ToolCallID:      "call_1",
		}},
		Continuations: []runs.Continuation{{
			RunID: "run_1", MemberID: "member_1", RunCreatedAt: time.Unix(1, 0).UTC(),
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
	if root.MemberID != "member_1" || len(got.Interrupts) != 1 || got.Interrupts[0].ItemID != "item_approval" ||
		got.Bindings[0].ToolCallID != "call_1" ||
		got.Interrupts[0].Approval == nil || got.Interrupts[0].Approval.Risk != tool.RiskHigh ||
		got.Interrupts[0].Approval.Tool.Name != "shell" ||
		got.Interrupts[0].Approval.Tool.Arguments.Canonical() != arguments.Canonical() ||
		got.Interrupts[0].Approval.Reason != "executes tests" || !got.Interrupts[0].Approval.Rememberable {
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
	pending := runs.Pending{
		RootRunID: "run_1", SessionID: "ses_a", ExecutorID: "turn_1",
		Capabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_question", ItemOccurredAt: time.Unix(2, 0).UTC(), RunID: "run_1", Kind: interrupt.Question,
			Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}},
		}},
		Bindings: []runs.InterruptBinding{{
			InterruptItemID: "item_question", MemberID: "member_root", RequestID: "request_root",
		}},
		Continuations: []runs.Continuation{{
			RunID: "run_1", MemberID: "member_root", RunCreatedAt: time.Unix(1, 0).UTC(),
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
	lineage := run.Lineage{
		SpawnedByItemID: "item_spawn_child",
		ParentRunID:     "run_root",
		RootRunID:       "run_root",
	}
	pending := runs.Pending{
		RootRunID:  "run_root",
		SessionID:  "session_1",
		ExecutorID: "turn_1",
		Capabilities: run.Capabilities{
			ChildRuns: true, InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_child", ItemOccurredAt: time.Unix(2, 0).UTC(), RunID: "run_child", Kind: interrupt.Question,
			Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}},
		}},
		Bindings: []runs.InterruptBinding{{
			InterruptItemID: "item_child",
			MemberID:        "member_child",
			RequestID:       "request_child",
		}},
		Continuations: []runs.Continuation{
			{
				RunID:        "run_child",
				MemberID:     "member_child",
				Lineage:      lineage,
				RunCreatedAt: createdAt,
				DrainedTools: []runs.DrainedTool{{
					ItemID: "item_open", ItemOccurredAt: createdAt.Add(time.Second),
					CallID: "call_open", SourceCallID: "provider_open",
					Name: "shell", Arguments: "{}",
				}},
			},
			{
				RunID:        "run_root",
				MemberID:     "member_root",
				RunCreatedAt: createdAt,
				CommittedTools: []runs.CommittedTool{{
					ItemID: "item_spawn_child", CallID: "call_child", SourceCallID: "provider_child",
					Name: "delegate_task", Arguments: "{}",
					Failure: tool.Failure{
						Kind:   tool.FailureChildRunCanceled,
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
	if child.Lineage != lineage || child.MemberID != "member_child" || len(child.DrainedTools) != 1 ||
		!child.DrainedTools[0].ItemOccurredAt.Equal(createdAt.Add(time.Second)) ||
		child.DrainedTools[0].SourceCallID != "provider_open" {
		t.Fatalf("child continuation = %+v, want lineage %+v", child, lineage)
	}
	root, found := got.RootContinuation()
	if !found ||
		len(root.CommittedTools) != 1 ||
		root.CommittedTools[0].ItemID != "item_spawn_child" ||
		root.CommittedTools[0].CallID != "call_child" ||
		root.CommittedTools[0].SourceCallID != "provider_child" ||
		root.CommittedTools[0].Failure.Kind != tool.FailureChildRunCanceled {
		t.Fatalf("root committed tools = %+v, want canceled child result hand-off", root.CommittedTools)
	}
}

func TestInterruptStoreRejectsUnknownExecutorTopologyFields(t *testing.T) {
	database, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	store := persistence.NewInterruptStore(sqlite.NewInterruptStore(database))
	pending := runs.Pending{
		RootRunID: "run_root", SessionID: "session_1", ExecutorID: "turn_1",
		Capabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_question", ItemOccurredAt: time.Unix(2, 0).UTC(), RunID: "run_root", Kind: interrupt.Question,
			Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}},
		}},
		Bindings: []runs.InterruptBinding{{
			InterruptItemID: "item_question", MemberID: "member_root", RequestID: "request_root",
		}},
		Continuations: []runs.Continuation{{
			RunID: "run_root", MemberID: "member_root", RunCreatedAt: time.Unix(1, 0).UTC(),
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
		`"memberId":"member_root"`,
		`"memberId":"member_root","parentProcessId":"legacy_parent"`,
		1,
	)
	if poisoned == encoded {
		t.Fatalf("continuation JSON does not contain member identity: %s", encoded)
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
		err := store.Open(ctx, runs.Pending{
			RootRunID: runID, SessionID: "ses_" + runID, ExecutorID: "turn_" + runID,
			Capabilities: run.Capabilities{
				InterruptKinds: []interrupt.Kind{interrupt.Question},
			},
			Interrupts: []transcript.Interrupt{{
				ItemID: "item_" + runID, ItemOccurredAt: time.Unix(2, 0).UTC(), RunID: runID,
				Kind: interrupt.Question, Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "continue?"}}},
			}},
			Bindings: []runs.InterruptBinding{{
				InterruptItemID: "item_" + runID,
				MemberID:        "member_shared",
				RequestID:       "request_" + runID,
			}},
			Continuations: []runs.Continuation{{
				RunID: runID, MemberID: "member_shared", RunCreatedAt: time.Unix(1, 0).UTC(),
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
