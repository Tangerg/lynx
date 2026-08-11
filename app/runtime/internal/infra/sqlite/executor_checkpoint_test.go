package sqlite_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
)

func newExecutorCheckpointStorage(t *testing.T) (*sql.DB, *persistence.ExecutorCheckpointStore) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
}

func storedExecutorCheckpoint(rootMemberID, sessionID, payload string) runs.ExecutorCheckpoint {
	selection, err := modelref.New("anthropic", "claude")
	if err != nil {
		panic(err)
	}
	return runs.ExecutorCheckpoint{
		RootMemberID: rootMemberID,
		Payload:      []byte(payload),
		BuildID:      "sha256:checkpoint-build",
		Scope: runs.ExecutionScope{
			SessionID:         sessionID,
			CWD:               "/workspace/" + sessionID,
			Isolated:          true,
			GoalIncarnationID: "lease-" + sessionID,
		},
		ModelSelection: selection,
		Limits: run.Limits{
			MaxTotalTokens: 8_192,
			MaxBudgetUSD:   2.5,
			MaxSteps:       16,
		},
		Capabilities: run.Capabilities{
			ChildRuns:      true,
			InterruptKinds: []interrupt.Kind{interrupt.Approval, interrupt.Question},
		},
		Usage: accounting.Snapshot{Models: []accounting.ModelUsage{{
			Model: "claude",
			TokenUsage: accounting.TokenUsage{
				PromptTokens: 12, CompletionTokens: 7, ReasoningTokens: 3,
				CacheReadTokens: 4, CacheWriteTokens: 2,
			},
			CostUSD: 0.25,
			Calls:   1,
		}}},
	}
}

func TestExecutorCheckpointStoreReplacesOneRootOwnedAggregate(t *testing.T) {
	db, store := newExecutorCheckpointStorage(t)
	ctx := t.Context()
	first := storedExecutorCheckpoint("member_root", "session-1", `{"tree":"first"}`)
	if err := store.SaveCheckpoint(ctx, first); err != nil {
		t.Fatalf("SaveCheckpoint(first): %v", err)
	}
	replacement := storedExecutorCheckpoint("member_root", first.Scope.SessionID, `{"tree":"replacement","children":["opaque"]}`)
	replacement.Usage.Models[0].Calls = 2
	if err := store.SaveCheckpoint(ctx, replacement); err != nil {
		t.Fatalf("SaveCheckpoint(replacement): %v", err)
	}

	got, err := store.LoadCheckpoint(ctx, replacement.RootMemberID)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if !reflect.DeepEqual(got, replacement) {
		t.Fatalf("checkpoint = %+v, want %+v", got, replacement)
	}
	var rows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM executor_checkpoints`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("checkpoint rows = %d, %v; want one aggregate", rows, err)
	}
}

func TestExecutorCheckpointStoreRejectsImmutablePolicyReplacement(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	first := storedExecutorCheckpoint("member_root", "session-1", `{"tree":"first"}`)
	if err := store.SaveCheckpoint(t.Context(), first); err != nil {
		t.Fatalf("SaveCheckpoint(first): %v", err)
	}
	for name, mutate := range map[string]func(*runs.ExecutorCheckpoint){
		"build":            func(checkpoint *runs.ExecutorCheckpoint) { checkpoint.BuildID = "other-build" },
		"cwd":              func(checkpoint *runs.ExecutorCheckpoint) { checkpoint.Scope.CWD = "/other" },
		"isolation":        func(checkpoint *runs.ExecutorCheckpoint) { checkpoint.Scope.Isolated = false },
		"goal incarnation": func(checkpoint *runs.ExecutorCheckpoint) { checkpoint.Scope.GoalIncarnationID = "other-lease" },
		"provider": func(checkpoint *runs.ExecutorCheckpoint) {
			checkpoint.ModelSelection, _ = modelref.New("openai", "claude")
		},
		"model": func(checkpoint *runs.ExecutorCheckpoint) {
			checkpoint.ModelSelection, _ = modelref.New("anthropic", "claude-sonnet")
		},
		"limits": func(checkpoint *runs.ExecutorCheckpoint) { checkpoint.Limits.MaxSteps++ },
		"capabilities": func(checkpoint *runs.ExecutorCheckpoint) {
			checkpoint.Capabilities.ChildRuns = false
		},
	} {
		t.Run(name, func(t *testing.T) {
			replacement := first.Clone()
			replacement.Payload = []byte(`{"tree":"replacement"}`)
			mutate(&replacement)
			if err := store.SaveCheckpoint(t.Context(), replacement); !errors.Is(err, runs.ErrInvalidExecutorCheckpoint) {
				t.Fatalf("SaveCheckpoint error = %v, want ErrInvalidExecutorCheckpoint", err)
			}
			stored, err := store.LoadCheckpoint(t.Context(), first.RootMemberID)
			if err != nil {
				t.Fatalf("LoadCheckpoint: %v", err)
			}
			if !reflect.DeepEqual(stored, first) {
				t.Fatalf("checkpoint after rejected replacement = %+v, want %+v", stored, first)
			}
		})
	}
}

func TestExecutorCheckpointStoreRejectsCumulativeUsageRegression(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	first := storedExecutorCheckpoint("member_root", "session-1", `{"tree":"first"}`)
	first.Usage.Models[0].Calls = 2
	if err := store.SaveCheckpoint(t.Context(), first); err != nil {
		t.Fatalf("SaveCheckpoint(first): %v", err)
	}
	replacement := first.Clone()
	replacement.Payload = []byte(`{"tree":"stale"}`)
	replacement.Usage.Models[0].Calls = 1
	if err := store.SaveCheckpoint(t.Context(), replacement); !errors.Is(err, runs.ErrInvalidExecutorCheckpoint) {
		t.Fatalf("SaveCheckpoint(regression) error = %v, want ErrInvalidExecutorCheckpoint", err)
	}
	stored, err := store.LoadCheckpoint(t.Context(), first.RootMemberID)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if !reflect.DeepEqual(stored, first) {
		t.Fatalf("checkpoint after usage regression = %+v, want %+v", stored, first)
	}
}

func TestExecutorCheckpointStoreRejectsOwnerReassignment(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	first := storedExecutorCheckpoint("member_root", "session-1", `{"tree":"first"}`)
	if err := store.SaveCheckpoint(t.Context(), first); err != nil {
		t.Fatalf("SaveCheckpoint(first): %v", err)
	}
	replacement := storedExecutorCheckpoint(first.RootMemberID, "session-2", `{"tree":"replacement"}`)
	if err := store.SaveCheckpoint(t.Context(), replacement); !errors.Is(err, runs.ErrInvalidExecutorCheckpoint) {
		t.Fatalf("SaveCheckpoint(reassignment) error = %v, want ErrInvalidExecutorCheckpoint", err)
	}
	stored, err := store.LoadCheckpoint(t.Context(), first.RootMemberID)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if stored.Scope.SessionID != first.Scope.SessionID || !bytes.Equal(stored.Payload, first.Payload) {
		t.Fatalf("checkpoint after rejected reassignment = %+v, want original owner and payload", stored)
	}
}

func TestExecutorCheckpointStoreTreatsPayloadAsOpaqueBytes(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	checkpoint := storedExecutorCheckpoint("member_root", "session-1", "\x00not-json\xff")
	if err := store.SaveCheckpoint(t.Context(), checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	got, err := store.LoadCheckpoint(t.Context(), checkpoint.RootMemberID)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if !bytes.Equal(got.Payload, checkpoint.Payload) {
		t.Fatalf("payload = %q, want exact opaque bytes %q", got.Payload, checkpoint.Payload)
	}
}

func TestExecutorCheckpointStoreRoundTripsApplicationEnvelope(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	want := storedExecutorCheckpoint("member_root", "session-1", `{"opaque":true}`)
	if err := store.SaveCheckpoint(t.Context(), want); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	got, err := store.LoadCheckpoint(t.Context(), want.RootMemberID)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if got.RootMemberID != want.RootMemberID ||
		got.BuildID != want.BuildID ||
		got.Scope != want.Scope ||
		got.ModelSelection != want.ModelSelection ||
		got.Limits != want.Limits ||
		!reflect.DeepEqual(got.Capabilities, want.Capabilities) ||
		!reflect.DeepEqual(got.Usage, want.Usage) {
		t.Fatalf("application envelope = %+v, want %+v", got, want)
	}
}

func TestExecutorCheckpointStoreRejectsUnversionedOrMalformedCapabilities(t *testing.T) {
	tests := map[string]string{
		"unversioned policy":     `{"scope":{"session_id":"session-1","cwd":"/workspace/session-1","workspace_cwd":"","isolated":true,"goal_incarnation_id":"lease-session-1"},"provider":"anthropic","model":"claude","limits":{"max_total_tokens":8192,"max_budget_usd":2.5,"max_steps":16},"capabilities":{"child_runs":true,"interrupt_kinds":["approval","question"]}}`,
		"retired lease field":    `{"schema_version":2,"scope":{"session_id":"session-1","cwd":"/workspace/session-1","workspace_cwd":"","isolated":true,"goal_lease_id":"lease-session-1"},"provider":"anthropic","model":"claude","limits":{"max_total_tokens":8192,"max_budget_usd":2.5,"max_steps":16},"capabilities":{"child_runs":true,"interrupt_kinds":["approval","question"]}}`,
		"retired policy version": `{"schema_version":1,"scope":{"session_id":"session-1","cwd":"/workspace/session-1","workspace_cwd":"","isolated":true,"goal_incarnation_id":"lease-session-1"},"provider":"anthropic","model":"claude","limits":{"max_total_tokens":8192,"max_budget_usd":2.5,"max_steps":16},"capabilities":{"child_runs":true,"interrupt_kinds":["approval","question"]}}`,
		"missing capability set": `{"schema_version":2,"scope":{"session_id":"session-1","cwd":"/workspace/session-1","workspace_cwd":"","isolated":true,"goal_incarnation_id":"lease-session-1"},"provider":"anthropic","model":"claude","limits":{"max_total_tokens":8192,"max_budget_usd":2.5,"max_steps":16}}`,
		"noncanonical kinds":     `{"schema_version":2,"scope":{"session_id":"session-1","cwd":"/workspace/session-1","workspace_cwd":"","isolated":true,"goal_incarnation_id":"lease-session-1"},"provider":"anthropic","model":"claude","limits":{"max_total_tokens":8192,"max_budget_usd":2.5,"max_steps":16},"capabilities":{"child_runs":true,"interrupt_kinds":["question","approval"]}}`,
		"unknown kind":           `{"schema_version":2,"scope":{"session_id":"session-1","cwd":"/workspace/session-1","workspace_cwd":"","isolated":true,"goal_incarnation_id":"lease-session-1"},"provider":"anthropic","model":"claude","limits":{"max_total_tokens":8192,"max_budget_usd":2.5,"max_steps":16},"capabilities":{"child_runs":true,"interrupt_kinds":["approval","future"]}}`,
	}
	for name, policy := range tests {
		t.Run(name, func(t *testing.T) {
			db, store := newExecutorCheckpointStorage(t)
			checkpoint := storedExecutorCheckpoint("member_root", "session-1", `{"opaque":true}`)
			if err := store.SaveCheckpoint(t.Context(), checkpoint); err != nil {
				t.Fatalf("SaveCheckpoint: %v", err)
			}
			if _, err := db.ExecContext(t.Context(),
				`UPDATE executor_checkpoints SET policy = ? WHERE root_member_id = ?`,
				policy,
				checkpoint.RootMemberID,
			); err != nil {
				t.Fatalf("corrupt policy: %v", err)
			}
			if _, err := store.LoadCheckpoint(t.Context(), checkpoint.RootMemberID); !errors.Is(err, runs.ErrInvalidExecutorCheckpoint) {
				t.Fatalf("LoadCheckpoint error = %v, want ErrInvalidExecutorCheckpoint", err)
			}
		})
	}
}

func TestExecutorCheckpointStoreMissingUsesDomainSentinel(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	_, err := store.LoadCheckpoint(t.Context(), "missing")
	if !errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("LoadCheckpoint error = %v, want ErrExecutorCheckpointNotFound", err)
	}
}

func TestExecutorCheckpointStoreRejectsInvalidEnvelopeBeforeMutation(t *testing.T) {
	db, store := newExecutorCheckpointStorage(t)
	checkpoint := storedExecutorCheckpoint("member_root", "session-1", `{"opaque":true}`)
	checkpoint.BuildID = ""
	if err := store.SaveCheckpoint(t.Context(), checkpoint); !errors.Is(err, runs.ErrInvalidExecutorCheckpoint) {
		t.Fatalf("SaveCheckpoint error = %v, want ErrInvalidExecutorCheckpoint", err)
	}
	var rows int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM executor_checkpoints`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("checkpoint rows after rejection = %d, %v", rows, err)
	}
}

func TestExecutorCheckpointStoreDeletesExactAggregates(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	ctx := t.Context()
	for _, checkpoint := range []runs.ExecutorCheckpoint{
		storedExecutorCheckpoint("root-a", "session-a", `{"root":"a"}`),
		storedExecutorCheckpoint("root-b", "session-a", `{"root":"b"}`),
		storedExecutorCheckpoint("root-c", "session-b", `{"root":"c"}`),
	} {
		if err := store.SaveCheckpoint(ctx, checkpoint); err != nil {
			t.Fatalf("SaveCheckpoint(%s): %v", checkpoint.RootMemberID, err)
		}
	}
	if err := store.DeleteCheckpoints(ctx, "session-a", []string{"root-b", "unknown"}); err != nil {
		t.Fatalf("DeleteCheckpoints: %v", err)
	}
	if _, err := store.LoadCheckpoint(ctx, "root-b"); !errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("deleted root-b = %v", err)
	}
	for _, rootID := range []string{"root-a", "root-c"} {
		if _, err := store.LoadCheckpoint(ctx, rootID); err != nil {
			t.Fatalf("unrelated checkpoint %q: %v", rootID, err)
		}
	}
	if err := store.DeleteCheckpoints(ctx, "session-a", nil); err == nil {
		t.Fatal("DeleteCheckpoints accepted an empty owner set")
	}
	if err := store.DeleteCheckpoints(ctx, "session-a", []string{"root-a", "root-a"}); err == nil {
		t.Fatal("DeleteCheckpoints accepted duplicate roots")
	}
}

func TestExecutorCheckpointStoreRejectsForeignSessionDeletion(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	checkpoint := storedExecutorCheckpoint("root-a", "session-a", `{"root":"a"}`)
	if err := store.SaveCheckpoint(t.Context(), checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	err := store.DeleteCheckpoints(t.Context(), "session-b", []string{checkpoint.RootMemberID})
	if !errors.Is(err, runs.ErrInvalidExecutorCheckpoint) {
		t.Fatalf("DeleteCheckpoints error = %v, want ErrInvalidExecutorCheckpoint", err)
	}
	if _, err := store.LoadCheckpoint(t.Context(), checkpoint.RootMemberID); err != nil {
		t.Fatalf("foreign checkpoint was deleted: %v", err)
	}
}

func TestExecutorCheckpointStoreDeletesByApplicationOwnership(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	ctx := context.Background()
	for _, checkpoint := range []runs.ExecutorCheckpoint{
		storedExecutorCheckpoint("keep", "session-a", `{"root":"keep"}`),
		storedExecutorCheckpoint("drop-session", "session-b", `{"root":"drop-session"}`),
		storedExecutorCheckpoint("drop-unowned", "session-c", `{"root":"drop-unowned"}`),
	} {
		if err := store.SaveCheckpoint(ctx, checkpoint); err != nil {
			t.Fatalf("SaveCheckpoint(%s): %v", checkpoint.RootMemberID, err)
		}
	}
	if err := store.DeleteSessionCheckpoints(ctx, "session-b"); err != nil {
		t.Fatalf("DeleteSessionCheckpoints: %v", err)
	}
	if err := store.DeleteUnownedCheckpoints(ctx, []string{"keep"}); err != nil {
		t.Fatalf("DeleteUnownedCheckpoints: %v", err)
	}
	for _, rootID := range []string{"drop-session", "drop-unowned"} {
		if _, err := store.LoadCheckpoint(ctx, rootID); !errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
			t.Fatalf("stale checkpoint %q = %v", rootID, err)
		}
	}
	if got, err := store.LoadCheckpoint(ctx, "keep"); err != nil || got.RootMemberID != "keep" {
		t.Fatalf("preserved checkpoint = (%+v, %v)", got, err)
	}
}

func TestExecutorCheckpointSchemaContainsNoFrameworkTopology(t *testing.T) {
	db, _ := newExecutorCheckpointStorage(t)
	rows, err := db.Query(`PRAGMA table_info(executor_checkpoints)`)
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		columns = append(columns, name)
	}
	for _, leaked := range []string{"process_id", "parent_process_id", "started_at", "status", "suspension"} {
		if slices.Contains(columns, leaked) {
			t.Fatalf("executor checkpoint schema leaks framework column %q: %v", leaked, columns)
		}
	}
}
