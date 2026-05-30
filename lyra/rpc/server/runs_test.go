package server

import (
	"testing"

	"github.com/Tangerg/lynx/lyra/internal/service/chat"
)

func TestStopReasonToWire(t *testing.T) {
	cases := map[chat.TurnEndReason]string{
		chat.TurnEndCompleted:      "completed",
		chat.TurnEndCancelled:      "canceled",
		chat.TurnEndErrored:        "error",
		chat.TurnEndBudgetExceeded: "max_budget",
	}
	for r, want := range cases {
		if got := stopReasonToWire(r); got != want {
			t.Errorf("stopReasonToWire(%v) = %q, want %q", r, got, want)
		}
	}
}

// TestTurnEndToRunResult pins the engine→wire metering projection,
// including the "zero cost is omitted, not faked to 0" rule (API.md §6.3).
func TestTurnEndToRunResult(t *testing.T) {
	cost := 0.0123
	e := chat.TurnEnd{
		Reason:     chat.TurnEndCompleted,
		TokenUsage: chat.TokenUsage{PromptTokens: 100, CompletionTokens: 50, ReasoningTokens: 10},
		UsageByModel: []chat.ModelUsage{
			{Model: "claude-x", TokenUsage: chat.TokenUsage{PromptTokens: 100, CompletionTokens: 50}, CostUSD: cost},
		},
		CostUSD: cost,
	}
	res := turnEndToRunResult(e)
	if res.StopReason != "completed" {
		t.Fatalf("StopReason = %q, want completed", res.StopReason)
	}
	if res.Usage == nil || res.Usage.InputTokens != 100 || res.Usage.OutputTokens != 50 || res.Usage.ReasoningTokens != 10 {
		t.Fatalf("Usage = %+v", res.Usage)
	}
	if res.CostUSD == nil || *res.CostUSD != cost {
		t.Fatalf("CostUSD = %v, want %v", res.CostUSD, cost)
	}
	mu, ok := res.Usage.ByModel["claude-x"]
	if !ok || mu.InputTokens != 100 || mu.OutputTokens != 50 || mu.CostUSD == nil || *mu.CostUSD != cost {
		t.Fatalf("ByModel[claude-x] = %+v (ok=%v)", mu, ok)
	}

	// Zero cost (no pricing hook) → CostUSD omitted (nil), not 0.
	res2 := turnEndToRunResult(chat.TurnEnd{
		Reason:     chat.TurnEndBudgetExceeded,
		TokenUsage: chat.TokenUsage{PromptTokens: 5},
	})
	if res2.StopReason != "max_budget" {
		t.Errorf("StopReason = %q, want max_budget", res2.StopReason)
	}
	if res2.CostUSD != nil {
		t.Errorf("zero cost should be omitted (nil), got %v", *res2.CostUSD)
	}
}
