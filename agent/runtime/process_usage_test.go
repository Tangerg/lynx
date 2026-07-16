package runtime

import (
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

// newTestBudget produces an isolated processBudget with its own
// mutex so the call-history methods can be exercised without
// the surrounding Process setup.
func newTestBudget() *processBudget {
	return &processBudget{lock: new(sync.RWMutex)}
}

func TestProcessBudgetRecordModelCallAggregatesCostAndTokens(t *testing.T) {
	b := newTestBudget()

	b.recordModelCall(core.ModelCall{
		Model:            "claude-sonnet-4-5",
		Provider:         "anthropic",
		CostUSD:          0.012,
		PromptTokens:     1000,
		CompletionTokens: 500,
	})
	b.recordModelCall(core.ModelCall{
		Model:            "gpt-4o",
		Provider:         "openai",
		CostUSD:          0.008,
		PromptTokens:     400,
		CompletionTokens: 200,
	})

	cost, tokens, actions := b.usage(0)
	if cost-0.020 > 1e-9 || cost-0.020 < -1e-9 {
		t.Errorf("cost: want ~0.020, got %.6f", cost)
	}
	if tokens != 2100 { // 1500 + 600
		t.Errorf("tokens: want 2100, got %d", tokens)
	}
	if actions != 0 {
		t.Errorf("actions: want 0, got %d", actions)
	}

	history := b.modelCallHistory()
	if len(history) != 2 {
		t.Fatalf("history: want 2 entries, got %d", len(history))
	}
	if history[0].Model != "claude-sonnet-4-5" || history[1].Model != "gpt-4o" {
		t.Errorf("history order or content wrong: %#v", history)
	}
}

func TestProcessBudgetRecordEmbeddingCall(t *testing.T) {
	b := newTestBudget()

	b.recordEmbeddingCall(core.EmbeddingCall{
		Model:       "voyage-3",
		Provider:    "voyage",
		CostUSD:     0.001,
		InputTokens: 800,
		InputCount:  10,
	})
	b.recordEmbeddingCall(core.EmbeddingCall{
		Model:       "text-embedding-3-small",
		Provider:    "openai",
		CostUSD:     0.0005,
		InputTokens: 400,
		InputCount:  5,
	})

	history := b.embeddingCallHistory()
	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}

	cost, tokens, _ := b.usage(0)
	if cost-0.0015 > 1e-9 || cost-0.0015 < -1e-9 {
		t.Errorf("cost: want ~0.0015, got %.6f", cost)
	}
	if tokens != 1200 {
		t.Errorf("tokens: want 1200 (sum of InputTokens), got %d", tokens)
	}
}

func TestProcessBudgetRecordUsageAppendsToModelCallHistory(t *testing.T) {
	// The flat RecordUsage(cost, tokens) shape (now routed through
	// Process.RecordUsage → recordModelCall) appends a stub
	// ModelCall so per-call audit code sees a record even when the
	// integration layer doesn't know model / provider.
	b := newTestBudget()
	b.recordModelCall(core.ModelCall{CostUSD: 0.005, PromptTokens: 250})

	history := b.modelCallHistory()
	if len(history) != 1 {
		t.Fatalf("history len = %d, want 1", len(history))
	}
	if history[0].CostUSD != 0.005 || history[0].PromptTokens != 250 {
		t.Errorf("stub model call: got %#v", history[0])
	}
	if history[0].Model != "" || history[0].Provider != "" {
		t.Errorf("stub model call should have empty model/provider, got %#v", history[0])
	}
}

func TestProcessBudgetPreservesModelCallTimestamp(t *testing.T) {
	// Timestamp defaulting lives in Process.RecordModelCall
	// (covered by TestModelCallsPublishEvents); the budget
	// layer stores whatever it receives verbatim.
	b := newTestBudget()

	explicit := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	b.recordModelCall(core.ModelCall{Timestamp: explicit, CostUSD: 0.001})

	history := b.modelCallHistory()
	if !history[0].Timestamp.Equal(explicit) {
		t.Errorf("explicit timestamp not preserved: got %v", history[0].Timestamp)
	}
}
