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
