package kernel

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/kernel/accounting"
	"github.com/Tangerg/lynx/core/model/chat"
)

// TestEngine_RunChat_TokenUsageAccumulates verifies the per-turn
// usage roll-up sums across both LLM rounds (tool-call + final
// reply). ReasoningTokens come from a pointer field on chat.Usage;
// only the rounds that populate it should contribute to the total.
func TestEngine_RunChat_TokenUsageAccumulates(t *testing.T) {
	reasoning := int64(3)
	stub := newUsageStubModel(
		chat.Usage{PromptTokens: 10, CompletionTokens: 5},
		chat.Usage{PromptTokens: 20, CompletionTokens: 7, ReasoningTokens: &reasoning},
	)
	client, _ := chat.NewClient(stub)
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
// rate table itself is the caller's; here a stub rate of $1/token makes
// cost equal total prompt+completion tokens (30 + 12 = 42).
func TestEngine_RunChat_PricingFillsCost(t *testing.T) {
	reasoning := int64(3)
	stub := newUsageStubModel(
		chat.Usage{PromptTokens: 10, CompletionTokens: 5},
		chat.Usage{PromptTokens: 20, CompletionTokens: 7, ReasoningTokens: &reasoning},
	)
	client, _ := chat.NewClient(stub)
	pricing := func(_, _ string, u *chat.Usage) float64 {
		return float64(u.PromptTokens + u.CompletionTokens)
	}
	eng, err := New(context.Background(), Config{ChatClient: client, Pricing: pricing})
	if err != nil {
		t.Fatal(err)
	}

	out, err := eng.runTurnSync(context.Background(), TurnRequest{Message: "go"})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}
	if out.CostUSD != 42 {
		t.Errorf("CostUSD = %v, want 42", out.CostUSD)
	}
	if len(out.UsageByModel) != 1 || out.UsageByModel[0].CostUSD != 42 {
		t.Errorf("per-model cost = %+v, want one entry costing 42", out.UsageByModel)
	}
}

// TestEngine_RunChat_StopsOnBudget verifies the per-turn token
// ceiling halts the tool loop at a round boundary before the next
// LLM call and reports the partial result with StoppedOnBudget set.
// Round 1 (tool call) spends 15 tokens; with MaxBudget=10 the loop
// must stop there and never run round 2.
func TestEngine_RunChat_StopsOnBudget(t *testing.T) {
	stub := newUsageStubModel(
		chat.Usage{PromptTokens: 10, CompletionTokens: 5},  // round 1 -> total 15
		chat.Usage{PromptTokens: 99, CompletionTokens: 99}, // round 2 -> must NOT run
	)
	client, _ := chat.NewClient(stub)
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	out, err := eng.runTurnSync(context.Background(), TurnRequest{Message: "go", MaxBudget: 10})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}
	if !out.StoppedOnBudget {
		t.Error("expected StoppedOnBudget=true after exceeding MaxBudget")
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
		chat.Usage{PromptTokens: 10, CompletionTokens: 5},  // round 1 -> $15
		chat.Usage{PromptTokens: 99, CompletionTokens: 99}, // round 2 -> must NOT run
	)
	client, _ := chat.NewClient(stub)
	pricing := func(_, _ string, u *chat.Usage) float64 {
		return float64(u.PromptTokens + u.CompletionTokens)
	}
	eng, err := New(context.Background(), Config{ChatClient: client, Pricing: pricing})
	if err != nil {
		t.Fatal(err)
	}

	out, err := eng.runTurnSync(context.Background(), TurnRequest{Message: "go", MaxCostUSD: 10})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}
	if !out.StoppedOnBudget {
		t.Error("expected StoppedOnBudget=true after exceeding MaxCostUSD")
	}
	if out.CostUSD != 15 {
		t.Errorf("CostUSD = %v, want 15 (round 2 must not run)", out.CostUSD)
	}
}
