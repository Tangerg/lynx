package agentexec

import (
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/core/chat"
)

func TestUsageLedgerAggregatesByModelAndOwnsSnapshots(t *testing.T) {
	ledger := emptyUsageLedger()
	for _, call := range []struct {
		model string
		usage chat.Usage
		cost  float64
	}{
		{model: "alpha", usage: chat.Usage{InputTokens: 3, OutputTokens: 2}, cost: 0.2},
		{model: "beta", usage: chat.Usage{InputTokens: 5, OutputTokens: 1}, cost: 0.4},
		{model: "alpha", usage: chat.Usage{InputTokens: 7, OutputTokens: 4}, cost: 0.3},
	} {
		if err := ledger.record(&chat.Response{Model: call.model, Usage: call.usage}, call.cost); err != nil {
			t.Fatalf("record %q: %v", call.model, err)
		}
	}

	got := ledger.snapshot()
	want := accounting.Snapshot{Models: []accounting.ModelUsage{
		{Model: "alpha", TokenUsage: accounting.TokenUsage{PromptTokens: 10, CompletionTokens: 6}, CostUSD: 0.5, Calls: 2},
		{Model: "beta", TokenUsage: accounting.TokenUsage{PromptTokens: 5, CompletionTokens: 1}, CostUSD: 0.4, Calls: 1},
	}}
	if len(got.Models) != len(want.Models) {
		t.Fatalf("models = %+v, want %+v", got.Models, want.Models)
	}
	for index := range want.Models {
		if got.Models[index] != want.Models[index] {
			t.Fatalf("models[%d] = %+v, want %+v", index, got.Models[index], want.Models[index])
		}
	}

	got.Models[0].PromptTokens = 999
	if fresh := ledger.snapshot(); fresh.Models[0].PromptTokens != 10 {
		t.Fatalf("snapshot aliases ledger state: %+v", fresh.Models[0])
	}

	restored, err := newUsageLedger(want)
	if err != nil {
		t.Fatalf("restore ledger: %v", err)
	}
	if restoredSnapshot := restored.snapshot(); len(restoredSnapshot.Models) != 2 ||
		restoredSnapshot.Models[0] != want.Models[0] ||
		restoredSnapshot.Models[1] != want.Models[1] {
		t.Fatalf("restored snapshot = %+v, want %+v", restoredSnapshot, want)
	}
}

func TestUsageLedgerRecordsConcurrently(t *testing.T) {
	ledger := emptyUsageLedger()
	const calls = 100
	var group sync.WaitGroup
	group.Add(calls)
	for range calls {
		go func() {
			defer group.Done()
			if err := ledger.record(&chat.Response{
				Model: "shared",
				Usage: chat.Usage{InputTokens: 2, OutputTokens: 1},
			}, 0.25); err != nil {
				t.Errorf("record: %v", err)
			}
		}()
	}
	group.Wait()

	snapshot := ledger.snapshot()
	if len(snapshot.Models) != 1 {
		t.Fatalf("models = %+v, want one aggregate", snapshot.Models)
	}
	got := snapshot.Models[0]
	if got.PromptTokens != 2*calls || got.CompletionTokens != calls ||
		got.CostUSD != 0.25*calls || got.Calls != calls {
		t.Fatalf("usage = %+v, want %d calls", got, calls)
	}
}

func TestUsageLedgerRejectsCrossModelOverflowAtomically(t *testing.T) {
	ledger, err := newUsageLedger(accounting.Snapshot{Models: []accounting.ModelUsage{{
		Model:      "first",
		TokenUsage: accounting.TokenUsage{PromptTokens: int64(math.MaxInt)},
		Calls:      1,
	}}})
	if err != nil {
		t.Fatalf("newUsageLedger: %v", err)
	}
	before := ledger.snapshot()
	err = ledger.record(&chat.Response{
		Model: "second",
		Usage: chat.Usage{InputTokens: 1},
	}, 0)
	if err == nil {
		t.Fatal("cross-model overflow was accepted")
	}
	after := ledger.snapshot()
	if len(after.Models) != 1 || after.Models[0] != before.Models[0] {
		t.Fatalf("overflow partially mutated ledger: before=%+v after=%+v", before, after)
	}
}

func TestUsageLedgerRejectsCostCapacityLossAtomically(t *testing.T) {
	ledger, err := newUsageLedger(accounting.Snapshot{Models: []accounting.ModelUsage{{
		Model:   "first",
		CostUSD: math.MaxFloat64,
		Calls:   1,
	}}})
	if err != nil {
		t.Fatalf("newUsageLedger: %v", err)
	}
	before := ledger.snapshot()
	err = ledger.record(&chat.Response{Model: "second"}, 1)
	if err == nil {
		t.Fatal("cost increment beyond representable capacity was accepted")
	}
	after := ledger.snapshot()
	if len(after.Models) != 1 || after.Models[0] != before.Models[0] {
		t.Fatalf("rejected cost partially mutated ledger: before=%+v after=%+v", before, after)
	}
}

func TestValidateCheckpointUsageRequiresOneConsistentCommit(t *testing.T) {
	startedAt := time.Date(2026, time.July, 27, 10, 0, 0, 0, time.UTC)
	root := core.ProcessSnapshot{
		SchemaVersion: core.ProcessSnapshotSchemaVersion,
		ID:            "root",
		Deployment:    core.DeploymentRef{Name: "chat", Digest: "root-digest"},
		StartedAt:     startedAt,
		Status:        core.StatusCompleted,
		OwnUsage:      core.Usage{Cost: 0.25, Tokens: 5, ModelCalls: 1},
	}
	child := core.ProcessSnapshot{
		SchemaVersion: core.ProcessSnapshotSchemaVersion,
		ID:            "child",
		ParentID:      root.ID,
		Deployment:    core.DeploymentRef{Name: "subtask", Digest: "child-digest"},
		StartedAt:     startedAt,
		Status:        core.StatusCompleted,
		OwnUsage:      core.Usage{Cost: 0.5, Tokens: 7, ModelCalls: 1},
	}
	tree := core.ProcessSnapshotTree{RootID: root.ID, Snapshots: []core.ProcessSnapshot{root, child}}
	usage := accounting.Snapshot{Models: []accounting.ModelUsage{{
		Model:      "model",
		TokenUsage: accounting.TokenUsage{PromptTokens: 8, CompletionTokens: 4},
		CostUSD:    0.75,
		Calls:      2,
	}}}
	if err := validateCheckpointUsage(tree, usage); err != nil {
		t.Fatalf("matching checkpoint: %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*accounting.Snapshot)
	}{
		{name: "tokens", mutate: func(snapshot *accounting.Snapshot) { snapshot.Models[0].PromptTokens++ }},
		{name: "cost", mutate: func(snapshot *accounting.Snapshot) { snapshot.Models[0].CostUSD += 0.1 }},
		{name: "calls", mutate: func(snapshot *accounting.Snapshot) { snapshot.Models[0].Calls++ }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := accounting.Snapshot{Models: append([]accounting.ModelUsage(nil), usage.Models...)}
			test.mutate(&candidate)
			if err := validateCheckpointUsage(tree, candidate); !errors.Is(err, core.ErrInvalidSnapshot) {
				t.Fatalf("error = %v, want ErrInvalidSnapshot", err)
			}
		})
	}
}
