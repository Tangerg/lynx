package model_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
)

func TestUsage_TotalTokensSumsPromptAndCompletion(t *testing.T) {
	u := &model.Usage{PromptTokens: 100, CompletionTokens: 50}
	if got := u.TotalTokens(); got != 150 {
		t.Fatalf("TotalTokens = %d, want 150", got)
	}
}

func TestUsage_TotalTokensExcludesBreakdownFields(t *testing.T) {
	// Breakdown values overlap with prompt/completion totals, so adding
	// them again would double-count the same tokens.
	reasoning := int64(20)
	cacheRead := int64(40)
	cacheWrite := int64(10)

	u := &model.Usage{
		PromptTokens:          100,
		CompletionTokens:      50,
		ReasoningTokens:       &reasoning,
		CacheReadInputTokens:  &cacheRead,
		CacheWriteInputTokens: &cacheWrite,
	}

	if got := u.TotalTokens(); got != 150 {
		t.Fatalf("TotalTokens = %d, want 150 (breakdowns must not be added)", got)
	}
}

func TestUsage_TotalTokensNilReceiver(t *testing.T) {
	var u *model.Usage
	if got := u.TotalTokens(); got != 0 {
		t.Fatalf("nil Usage TotalTokens = %d, want 0", got)
	}
}

func TestUsage_HasReasoningTokens(t *testing.T) {
	t.Run("nil usage", func(t *testing.T) {
		var u *model.Usage
		if u.HasReasoningTokens() {
			t.Fatal("nil usage should not report reasoning breakdown")
		}
	})

	t.Run("nil pointer means missing", func(t *testing.T) {
		u := &model.Usage{}
		if u.HasReasoningTokens() {
			t.Fatal("nil ReasoningTokens pointer should be reported as missing")
		}
	})

	t.Run("zero value still counts as reported", func(t *testing.T) {
		zero := int64(0)
		u := &model.Usage{ReasoningTokens: &zero}
		if !u.HasReasoningTokens() {
			t.Fatal("explicit zero must be distinguished from absent breakdown")
		}
	})
}

func TestUsage_HasCacheReadInputTokens(t *testing.T) {
	zero := int64(0)
	cases := []struct {
		name string
		u    *model.Usage
		want bool
	}{
		{name: "nil usage", u: nil, want: false},
		{name: "absent", u: &model.Usage{}, want: false},
		{name: "zero reported", u: &model.Usage{CacheReadInputTokens: &zero}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.u.HasCacheReadInputTokens(); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUsage_HasCacheWriteInputTokens(t *testing.T) {
	zero := int64(0)
	cases := []struct {
		name string
		u    *model.Usage
		want bool
	}{
		{name: "nil usage", u: nil, want: false},
		{name: "absent", u: &model.Usage{}, want: false},
		{name: "zero reported", u: &model.Usage{CacheWriteInputTokens: &zero}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.u.HasCacheWriteInputTokens(); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
