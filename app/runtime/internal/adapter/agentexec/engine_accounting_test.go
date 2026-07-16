package agentexec

import (
	"context"
	"math"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

// TestEngine_RunChat_TokenUsageAccumulates verifies the per-turn
// usage roll-up sums across both LLM rounds (tool-call + final
// reply). ReasoningTokens come from a pointer field on chat.Usage;
// only the rounds that populate it should contribute to the total.
func TestEngine_RunChat_TokenUsageAccumulates(t *testing.T) {
	reasoning := int64(3)
	stub := newUsageStubModel(
		chat.Usage{InputTokens: 10, OutputTokens: 5},
		chat.Usage{InputTokens: 20, OutputTokens: 7, ReasoningTokens: &reasoning},
	)
	client, _ := chatclient.New(stub)
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	out, err := eng.runTurnSync(context.Background(), TurnRequest{Message: "go"})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}
	got := out.Usage
	want := accounting.TokenUsage{PromptTokens: 30, CompletionTokens: 12, ReasoningTokens: 3}
	if got != want {
		t.Errorf("usage = %+v, want %+v", got, want)
	}
	// Usage is read back from the process invocation ledger, and the
	// per-model breakdown rolls up to the same total under the one
	// served model.
	if len(out.UsageByModel) != 1 ||
		out.UsageByModel[0].Model != "stub-usage-model" ||
		out.UsageByModel[0].TokenUsage != want {
		t.Errorf("UsageByModel = %+v, want one entry {stub-usage-model, %+v}", out.UsageByModel, want)
	}
}

// TestEngine_RunChat_PricingFillsCost verifies the cost conduit: with a
// Pricing hook configured, each round's cost is recorded on its
// invocation and rolls up to TurnOutput.CostUSD + per-model cost. The
// rate table itself is the caller's; here a stub rate of $0.01/token makes
// cost equal $0.42 while remaining below the process's default $2 ceiling.
func TestEngine_RunChat_PricingFillsCost(t *testing.T) {
	reasoning := int64(3)
	stub := newUsageStubModel(
		chat.Usage{InputTokens: 10, OutputTokens: 5},
		chat.Usage{InputTokens: 20, OutputTokens: 7, ReasoningTokens: &reasoning},
	)
	client, _ := chatclient.New(stub)
	pricing := func(_, _ string, u *chat.Usage) float64 {
		return float64(u.InputTokens+u.OutputTokens) / 100
	}
	eng, err := New(context.Background(), Config{ChatClient: client, Pricing: pricing})
	if err != nil {
		t.Fatal(err)
	}

	out, err := eng.runTurnSync(context.Background(), TurnRequest{Message: "go"})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}
	if math.Abs(out.CostUSD-0.42) > 1e-9 {
		t.Errorf("CostUSD = %v, want 0.42", out.CostUSD)
	}
	if len(out.UsageByModel) != 1 || math.Abs(out.UsageByModel[0].CostUSD-0.42) > 1e-9 {
		t.Errorf("per-model cost = %+v, want one entry costing 0.42", out.UsageByModel)
	}
}

func TestEngine_TaskDelegationInheritsPerRunModelAndProvider(t *testing.T) {
	defaultClient, _ := chatclient.New(newNamedStub("default-model"))
	selectedModel := newDelegatingAccountingStub("selected-model", chat.Usage{InputTokens: 1, OutputTokens: 1})
	selectedClient, _ := chatclient.New(selectedModel)
	built, err := toolset.Build(t.Context(), toolset.BuildConfig{})
	if err != nil {
		t.Fatalf("toolset.Build: %v", err)
	}
	cleanupBuiltTools(t, built)
	var (
		mu        sync.Mutex
		providers []string
	)
	engine, err := New(t.Context(), Config{
		ChatClient:   defaultClient,
		Provider:     "default-provider",
		ToolResolver: built.Resolver,
		Pricing: func(provider, _ string, _ *chat.Usage) float64 {
			mu.Lock()
			providers = append(providers, provider)
			mu.Unlock()
			return 0.25
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	output, err := engine.runTurnSync(t.Context(), TurnRequest{
		Message:    "delegate this",
		Provider:   "selected-provider",
		ChatClient: selectedClient,
	})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}
	if output.Reply != "main: subtask done" || selectedModel.Calls() != 3 {
		t.Fatalf("reply/calls = %q/%d, want full three-call selected-model delegation", output.Reply, selectedModel.Calls())
	}
	if output.Usage.Total() != 6 || output.CostUSD != 0.75 {
		t.Fatalf("usage/cost = %d/%v, want 6/0.75 including child", output.Usage.Total(), output.CostUSD)
	}
	if len(output.UsageByModel) != 1 || output.UsageByModel[0].Model != "selected-model" {
		t.Fatalf("UsageByModel = %+v, want only selected-model", output.UsageByModel)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(providers) != 3 {
		t.Fatalf("pricing providers = %v, want three selected-provider calls", providers)
	}
	for _, provider := range providers {
		if provider != "selected-provider" {
			t.Fatalf("pricing providers = %v, child fell back to default provider", providers)
		}
	}
}

func TestEngine_TaskDelegationDoesNotStartChildAfterTokenBudgetIsSpent(t *testing.T) {
	model := newDelegatingAccountingStub("budget-model", chat.Usage{InputTokens: 1, OutputTokens: 1})
	client, _ := chatclient.New(model)
	engine := mustEngineWith(t, client, toolset.BuildConfig{})
	defer engine.Close()

	output, err := engine.runTurnSync(t.Context(), TurnRequest{
		Message:   "delegate this",
		MaxBudget: 2,
	})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}
	if output.StopReason != StopReasonBudget || output.Usage.Total() != 2 {
		t.Fatalf("stop/usage = %q/%d, want budget/2", output.StopReason, output.Usage.Total())
	}
	if model.Calls() != 1 {
		t.Fatalf("model calls = %d, want only the root call before child budget exhaustion", model.Calls())
	}
}

func TestEngine_TaskDelegationDoesNotStartChildAfterCostBudgetIsSpent(t *testing.T) {
	model := newDelegatingAccountingStub("cost-model", chat.Usage{InputTokens: 1, OutputTokens: 1})
	client, _ := chatclient.New(model)
	built, err := toolset.Build(t.Context(), toolset.BuildConfig{})
	if err != nil {
		t.Fatalf("toolset.Build: %v", err)
	}
	cleanupBuiltTools(t, built)
	engine, err := New(t.Context(), Config{
		ChatClient:   client,
		ToolResolver: built.Resolver,
		Pricing: func(_, _ string, _ *chat.Usage) float64 {
			return 1
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	output, err := engine.runTurnSync(t.Context(), TurnRequest{
		Message:    "delegate this",
		MaxCostUSD: 1,
	})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}
	if output.StopReason != StopReasonBudget || output.CostUSD != 1 {
		t.Fatalf("stop/cost = %q/%v, want budget/1", output.StopReason, output.CostUSD)
	}
	if model.Calls() != 1 {
		t.Fatalf("model calls = %d, want only the root call before child cost exhaustion", model.Calls())
	}
}

func TestEngine_TaskDelegationCountsChildCallsAgainstStepLimit(t *testing.T) {
	model := newDelegatingAccountingStub("steps-model", chat.Usage{})
	client, _ := chatclient.New(model)
	engine := mustEngineWith(t, client, toolset.BuildConfig{})
	defer engine.Close()

	output, err := engine.runTurnSync(t.Context(), TurnRequest{
		Message:  "delegate this",
		MaxSteps: 2,
	})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}
	if output.StopReason != StopReasonSteps {
		t.Fatalf("StopReason = %q, want steps", output.StopReason)
	}
	if model.Calls() != 2 {
		t.Fatalf("model calls = %d, want root + child with no third root call", model.Calls())
	}
}

// TestEngine_RunChat_StopsOnBudget verifies the per-turn token
// ceiling halts the tool loop at a round boundary before the next
// LLM call and reports the partial result with StopReasonBudget set.
// Round 1 (tool call) spends 15 tokens; with MaxBudget=10 the loop
// must stop there and never run round 2.
func TestEngine_RunChat_StopsOnBudget(t *testing.T) {
	stub := newUsageStubModel(
		chat.Usage{InputTokens: 10, OutputTokens: 5},  // round 1 -> total 15
		chat.Usage{InputTokens: 99, OutputTokens: 99}, // round 2 -> must NOT run
	)
	client, _ := chatclient.New(stub)
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	out, err := eng.runTurnSync(context.Background(), TurnRequest{Message: "go", MaxBudget: 10})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}
	if out.StopReason != StopReasonBudget {
		t.Errorf("StopReason = %q, want %q", out.StopReason, StopReasonBudget)
	}
	if got := out.Usage.Total(); got != 15 {
		t.Errorf("usage total = %d, want 15 (round 2 must not run)", got)
	}
}

// TestEngine_RunChat_StopsOnCostBudget verifies the dollar ceiling
// (MaxCostUSD) halts the loop the same way the token one does. With a
// $1/token stub rate, round 1 costs $15; MaxCostUSD=10 must stop there
// and never run round 2.
func TestEngine_RunChat_StopsOnCostBudget(t *testing.T) {
	stub := newUsageStubModel(
		chat.Usage{InputTokens: 10, OutputTokens: 5},  // round 1 -> $15
		chat.Usage{InputTokens: 99, OutputTokens: 99}, // round 2 -> must NOT run
	)
	client, _ := chatclient.New(stub)
	pricing := func(_, _ string, u *chat.Usage) float64 {
		return float64(u.InputTokens + u.OutputTokens)
	}
	eng, err := New(context.Background(), Config{ChatClient: client, Pricing: pricing})
	if err != nil {
		t.Fatal(err)
	}

	out, err := eng.runTurnSync(context.Background(), TurnRequest{Message: "go", MaxCostUSD: 10})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}
	if out.StopReason != StopReasonBudget {
		t.Errorf("StopReason = %q, want %q", out.StopReason, StopReasonBudget)
	}
	if out.CostUSD != 15 {
		t.Errorf("CostUSD = %v, want 15 (round 2 must not run)", out.CostUSD)
	}
}

func TestStopReason_Valid(t *testing.T) {
	for _, reason := range []StopReason{StopReasonNone, StopReasonBudget, StopReasonSteps} {
		if !reason.Valid() {
			t.Errorf("%q.Valid() = false, want true", reason)
		}
	}
	if invalid := StopReason("budget+steps"); invalid.Valid() {
		t.Errorf("%q.Valid() = true, want false", invalid)
	}
}
