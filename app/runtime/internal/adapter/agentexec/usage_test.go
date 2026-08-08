package agentexec

import (
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/core/chat"
)

func TestUsageLedgerAggregatesByModelAndOwnsSnapshots(t *testing.T) {
	ledger := emptyUsageLedger()
	process := ProcessRef{ID: "root"}
	for _, call := range []struct {
		model string
		usage chat.Usage
		cost  float64
	}{
		{model: "alpha", usage: chat.Usage{InputTokens: 3, OutputTokens: 2}, cost: 0.2},
		{model: "beta", usage: chat.Usage{InputTokens: 5, OutputTokens: 1}, cost: 0.4},
		{model: "alpha", usage: chat.Usage{InputTokens: 7, OutputTokens: 4}, cost: 0.3},
	} {
		if err := ledger.record(process, &chat.Response{Model: call.model, Usage: call.usage}, call.cost); err != nil {
			t.Fatalf("record %q: %v", call.model, err)
		}
	}

	got, err := ledger.snapshot()
	if err != nil {
		t.Fatal(err)
	}
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
	fresh, err := ledger.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Models[0].PromptTokens != 10 {
		t.Fatalf("snapshot aliases ledger state: %+v", fresh.Models[0])
	}

	restored, err := newUsageLedger(want)
	if err != nil {
		t.Fatalf("restore ledger: %v", err)
	}
	restoredSnapshot, err := restored.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(restoredSnapshot.Models) != 2 ||
		restoredSnapshot.Models[0] != want.Models[0] ||
		restoredSnapshot.Models[1] != want.Models[1] {
		t.Fatalf("restored snapshot = %+v, want %+v", restoredSnapshot, want)
	}
}

func TestUsageLedgerRecordsConcurrently(t *testing.T) {
	ledger := emptyUsageLedger()
	process := ProcessRef{ID: "root"}
	const calls = 100
	var group sync.WaitGroup
	group.Add(calls)
	for range calls {
		go func() {
			defer group.Done()
			if err := ledger.record(process, &chat.Response{
				Model: "shared",
				Usage: chat.Usage{InputTokens: 2, OutputTokens: 1},
			}, 0.25); err != nil {
				t.Errorf("record: %v", err)
			}
		}()
	}
	group.Wait()

	snapshot, err := ledger.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Models) != 1 {
		t.Fatalf("models = %+v, want one aggregate", snapshot.Models)
	}
	got := snapshot.Models[0]
	if got.PromptTokens != 2*calls || got.CompletionTokens != calls ||
		got.CostUSD != 0.25*calls || got.Calls != calls {
		t.Fatalf("usage = %+v, want %d calls", got, calls)
	}
}

func TestUsageLedgerProjectsExactProcessSubtrees(t *testing.T) {
	ledger := emptyUsageLedger()
	root := ProcessRef{ID: "root"}
	child := ProcessRef{ID: "child", ParentID: root.ID, SpawnCallID: "call-child"}
	grandchild := ProcessRef{ID: "grandchild", ParentID: child.ID, SpawnCallID: "call-grandchild"}
	sibling := ProcessRef{ID: "sibling", ParentID: root.ID, SpawnCallID: "call-sibling"}

	for _, process := range []ProcessRef{root, child, grandchild, sibling} {
		if err := ledger.register(process); err != nil {
			t.Fatalf("register %+v: %v", process, err)
		}
	}
	for _, call := range []struct {
		process ProcessRef
		model   string
		prompt  int64
		cost    float64
	}{
		{process: root, model: "root-model", prompt: 2, cost: 0.2},
		{process: child, model: "child-model", prompt: 3, cost: 0.3},
		{process: grandchild, model: "child-model", prompt: 5, cost: 0.5},
		{process: sibling, model: "sibling-model", prompt: 7, cost: 0.7},
	} {
		if err := ledger.record(call.process, &chat.Response{
			Model: call.model,
			Usage: chat.Usage{
				InputTokens:  call.prompt,
				OutputTokens: 1,
			},
		}, call.cost); err != nil {
			t.Fatalf("record %s: %v", call.process.ID, err)
		}
	}

	childOutput, err := ledger.output(child, "child reply", agent.InteractionStopModelCalls)
	if err != nil {
		t.Fatalf("child output: %v", err)
	}
	if childOutput.StopReason != agent.InteractionStopModelCalls ||
		childOutput.Usage.PromptTokens != 8 ||
		childOutput.Usage.CompletionTokens != 2 ||
		!sameCost(childOutput.CostUSD, 0.8) ||
		childOutput.Steps != 2 ||
		len(childOutput.UsageByModel) != 1 ||
		childOutput.UsageByModel[0].Model != "child-model" ||
		childOutput.UsageByModel[0].Calls != 2 {
		t.Fatalf("child subtree output = %+v", childOutput)
	}

	rootOutput, err := ledger.output(root, "root reply", agent.InteractionStopNone)
	if err != nil {
		t.Fatalf("root output: %v", err)
	}
	if rootOutput.Usage.PromptTokens != 17 ||
		rootOutput.Usage.CompletionTokens != 4 ||
		!sameCost(rootOutput.CostUSD, 1.7) ||
		rootOutput.Steps != 4 ||
		len(rootOutput.UsageByModel) != 3 {
		t.Fatalf("root subtree output = %+v", rootOutput)
	}
}

func TestUsageLedgerRejectsChangedProcessLineage(t *testing.T) {
	ledger := emptyUsageLedger()
	if err := ledger.register(ProcessRef{ID: "root"}); err != nil {
		t.Fatalf("register root: %v", err)
	}
	original := ProcessRef{ID: "child", ParentID: "root", SpawnCallID: "call-one"}
	if err := ledger.register(original); err != nil {
		t.Fatalf("register original: %v", err)
	}
	err := ledger.register(ProcessRef{
		ID:          original.ID,
		ParentID:    original.ParentID,
		SpawnCallID: "call-two",
	})
	if err == nil {
		t.Fatal("changed process lineage was accepted")
	}
	if _, snapshotErr := ledger.snapshot(); !errors.Is(snapshotErr, err) {
		t.Fatalf("snapshot error = %v, want latched %v", snapshotErr, err)
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
	before, err := ledger.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	err = ledger.record(ProcessRef{ID: "root"}, &chat.Response{
		Model: "second",
		Usage: chat.Usage{InputTokens: 1},
	}, 0)
	if err == nil {
		t.Fatal("cross-model overflow was accepted")
	}
	after, projectionErr := ledger.snapshot()
	if !errors.Is(projectionErr, err) {
		t.Fatalf("snapshot error = %v, want %v", projectionErr, err)
	}
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
	before, err := ledger.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	err = ledger.record(ProcessRef{ID: "root"}, &chat.Response{Model: "second"}, 1)
	if err == nil {
		t.Fatal("cost increment beyond representable capacity was accepted")
	}
	after, projectionErr := ledger.snapshot()
	if !errors.Is(projectionErr, err) {
		t.Fatalf("snapshot error = %v, want %v", projectionErr, err)
	}
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
