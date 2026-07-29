package accounting

import (
	"math"
	"testing"
)

func TestTokenUsageAdd(t *testing.T) {
	tests := []struct {
		name string
		base TokenUsage
		add  TokenUsage
		want TokenUsage
	}{
		{
			name: "empty rollup",
			add:  TokenUsage{PromptTokens: 10, CompletionTokens: 4, ReasoningTokens: 2, CacheReadTokens: 3, CacheWriteTokens: 1},
			want: TokenUsage{PromptTokens: 10, CompletionTokens: 4, ReasoningTokens: 2, CacheReadTokens: 3, CacheWriteTokens: 1},
		},
		{
			name: "existing rollup",
			base: TokenUsage{PromptTokens: 5, CompletionTokens: 2, ReasoningTokens: 1},
			add:  TokenUsage{PromptTokens: 7, CompletionTokens: 3, ReasoningTokens: 2},
			want: TokenUsage{PromptTokens: 12, CompletionTokens: 5, ReasoningTokens: 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.base
			got.Add(tt.add)
			if got != tt.want {
				t.Fatalf("TokenUsage = %+v, want %+v", got, tt.want)
			}
			if got.Total() != tt.want.PromptTokens+tt.want.CompletionTokens {
				t.Fatalf("Total() = %d, want %d", got.Total(), tt.want.PromptTokens+tt.want.CompletionTokens)
			}
		})
	}
}

func TestSnapshotTotalAggregatesModelsWithCapacityChecks(t *testing.T) {
	snapshot := Snapshot{Models: []ModelUsage{
		{
			Model: "alpha",
			TokenUsage: TokenUsage{
				PromptTokens:     3,
				CompletionTokens: 2,
				ReasoningTokens:  1,
			},
			CostUSD: 0.25,
			Calls:   1,
		},
		{
			Model: "beta",
			TokenUsage: TokenUsage{
				PromptTokens:     5,
				CompletionTokens: 1,
			},
			CostUSD: 0.5,
			Calls:   2,
		},
	}}
	total, err := snapshot.Total()
	if err != nil {
		t.Fatalf("Total: %v", err)
	}
	if total.PromptTokens != 8 ||
		total.CompletionTokens != 3 ||
		total.ReasoningTokens != 1 ||
		total.CostUSD != 0.75 ||
		total.Calls != 3 {
		t.Fatalf("total = %+v", total)
	}

	overflow := Snapshot{Models: []ModelUsage{
		{Model: "alpha", TokenUsage: TokenUsage{PromptTokens: math.MaxInt64}, Calls: 1},
		{Model: "beta", TokenUsage: TokenUsage{PromptTokens: 1}, Calls: 1},
	}}
	if _, err := overflow.Total(); err == nil {
		t.Fatal("overflowing snapshot aggregate was accepted")
	}
}
