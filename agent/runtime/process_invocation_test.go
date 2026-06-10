package runtime

import (
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

// newBudgetForTest produces an isolated processBudget with its own
// mutex so the invocation-history methods can be exercised without
// the surrounding AgentProcess setup.
func newBudgetForTest() *processBudget {
	return &processBudget{lock: new(sync.RWMutex)}
}

func TestProcessBudget_RecordLLMInvocation_AggregatesCostAndTokens(t *testing.T) {
	b := newBudgetForTest()

	b.recordLLMInvocation(core.LLMInvocation{
		Model:            "claude-sonnet-4-5",
		Provider:         "anthropic",
		CostUSD:          0.012,
		PromptTokens:     1000,
		CompletionTokens: 500,
	})
	b.recordLLMInvocation(core.LLMInvocation{
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

	history := b.llmHistory()
	if len(history) != 2 {
		t.Fatalf("history: want 2 entries, got %d", len(history))
	}
	if history[0].Model != "claude-sonnet-4-5" || history[1].Model != "gpt-4o" {
		t.Errorf("history order or content wrong: %#v", history)
	}
}

func TestProcessBudget_RecordEmbeddingInvocation(t *testing.T) {
	b := newBudgetForTest()

	b.recordEmbeddingInvocation(core.EmbeddingInvocation{
		Model:       "voyage-3",
		Provider:    "voyage",
		CostUSD:     0.001,
		InputTokens: 800,
		InputCount:  10,
	})
	b.recordEmbeddingInvocation(core.EmbeddingInvocation{
		Model:       "text-embedding-3-small",
		Provider:    "openai",
		CostUSD:     0.0005,
		InputTokens: 400,
		InputCount:  5,
	})

	history := b.embeddingHistory()
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

func TestProcessBudget_RecordUsage_AppendsToLLMHistory(t *testing.T) {
	// The flat RecordUsage(cost, tokens) shape (now routed through
	// AgentProcess.RecordUsage → recordLLMInvocation) appends a stub
	// LLMInvocation so per-call audit code sees a record even when the
	// integration layer doesn't know model / provider.
	b := newBudgetForTest()
	b.recordLLMInvocation(core.LLMInvocation{CostUSD: 0.005, PromptTokens: 250})

	history := b.llmHistory()
	if len(history) != 1 {
		t.Fatalf("history len = %d, want 1", len(history))
	}
	if history[0].CostUSD != 0.005 || history[0].PromptTokens != 250 {
		t.Errorf("stub invocation: got %#v", history[0])
	}
	if history[0].Model != "" || history[0].Provider != "" {
		t.Errorf("stub invocation should have empty model/provider, got %#v", history[0])
	}
}

func TestProcessBudget_LLMInvocation_TimestampPreserved(t *testing.T) {
	// Timestamp defaulting lives in AgentProcess.RecordLLMInvocation
	// (covered by TestRecordInvocation_PublishesEvents); the budget
	// layer stores whatever it receives verbatim.
	b := newBudgetForTest()

	explicit := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	b.recordLLMInvocation(core.LLMInvocation{Timestamp: explicit, CostUSD: 0.001})

	history := b.llmHistory()
	if !history[0].Timestamp.Equal(explicit) {
		t.Errorf("explicit timestamp not preserved: got %v", history[0].Timestamp)
	}
}
