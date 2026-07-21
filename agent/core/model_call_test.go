package core_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

func TestModelCallValidate(t *testing.T) {
	valid := core.ModelCall{Timestamp: time.Now(), PromptTokens: 10, CompletionTokens: 4, ReasoningTokens: 2}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid call: %v", err)
	}
	for name, call := range map[string]core.ModelCall{
		"zero timestamp":     {},
		"non-finite cost":    {Timestamp: time.Now(), CostUSD: math.NaN()},
		"reasoning overflow": {Timestamp: time.Now(), CompletionTokens: 1, ReasoningTokens: 2},
		"cache overflow":     {Timestamp: time.Now(), PromptTokens: 1, CacheReadInputTokens: 2},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call.Validate(); !errors.Is(err, core.ErrInvalidModelCall) {
				t.Fatalf("Validate error = %v, want ErrInvalidModelCall", err)
			}
		})
	}
}

func TestEmbeddingCallValidate(t *testing.T) {
	call := core.EmbeddingCall{Timestamp: time.Now(), InputTokens: -1}
	if err := call.Validate(); !errors.Is(err, core.ErrInvalidEmbeddingCall) {
		t.Fatalf("Validate error = %v, want ErrInvalidEmbeddingCall", err)
	}
}

func TestBudgetValidate(t *testing.T) {
	for _, budget := range []core.Budget{
		{CostLimit: math.Inf(1)},
		{ActionLimit: -1},
		{TokenLimit: -1},
	} {
		if err := budget.Validate(); !errors.Is(err, core.ErrInvalidBudget) {
			t.Fatalf("Validate(%#v) error = %v, want ErrInvalidBudget", budget, err)
		}
	}
}
