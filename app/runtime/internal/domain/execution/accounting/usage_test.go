package accounting

import "testing"

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

func TestBudget(t *testing.T) {
	tests := []struct {
		name          string
		budget        Budget
		tokens        int64
		costUSD       float64
		steps         int
		usageExceeded bool
		stepsExceeded bool
	}{
		{name: "zero value is unbounded", tokens: 100, costUSD: 10, steps: 5},
		{name: "below limits", budget: Budget{MaxTokens: 100, MaxCostUSD: 5, MaxSteps: 3}, tokens: 99, costUSD: 4.99, steps: 2},
		{name: "token boundary", budget: Budget{MaxTokens: 100}, tokens: 100, usageExceeded: true},
		{name: "cost boundary", budget: Budget{MaxCostUSD: 5}, costUSD: 5, usageExceeded: true},
		{name: "step boundary", budget: Budget{MaxSteps: 3}, steps: 3, stepsExceeded: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.budget.UsageExceeded(tt.tokens, tt.costUSD); got != tt.usageExceeded {
				t.Errorf("UsageExceeded() = %t, want %t", got, tt.usageExceeded)
			}
			if got := tt.budget.StepsExceeded(tt.steps); got != tt.stepsExceeded {
				t.Errorf("StepsExceeded() = %t, want %t", got, tt.stepsExceeded)
			}
		})
	}
}
