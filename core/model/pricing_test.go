package model

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestPricing_Cost(t *testing.T) {
	// Claude Sonnet-style rates ($/1M): in 3, out 15, cache read 0.3,
	// cache write 3.75.
	p := Pricing{InputPer1M: 3, OutputPer1M: 15, CacheReadPer1M: 0.3, CacheWritePer1M: 3.75}

	cases := []struct {
		name string
		u    *Usage
		want float64
	}{
		{
			name: "no cache",
			u:    &Usage{PromptTokens: 1000, CompletionTokens: 500},
			// 1000*3 + 500*15 = 10500 / 1e6
			want: 0.0105,
		},
		{
			name: "with cache read+write",
			// PromptTokens includes the 800 read + 100 write; 100 uncached.
			u: &Usage{
				PromptTokens:          1000,
				CompletionTokens:      500,
				CacheReadInputTokens:  new(int64(800)),
				CacheWriteInputTokens: new(int64(100)),
			},
			// 100*3 + 500*15 + 800*0.3 + 100*3.75 = 300+7500+240+375 = 8415 / 1e6
			want: 0.008415,
		},
		{name: "nil usage", u: nil, want: 0},
	}
	for _, c := range cases {
		if got := p.Cost(c.u); !approx(got, c.want) {
			t.Errorf("%s: Cost = %v, want %v", c.name, got, c.want)
		}
	}

	// Zero pricing (unknown) → always 0, even with real usage.
	if got := (Pricing{}).Cost(&Usage{PromptTokens: 1000, CompletionTokens: 500}); got != 0 {
		t.Errorf("zero pricing Cost = %v, want 0", got)
	}
	if !(Pricing{}).IsZero() || (Pricing{InputPer1M: 1}).IsZero() {
		t.Error("IsZero mismatch")
	}
}

// TestPricing_Cost_CacheRateFallback verifies that when a cache rate is
// unset (0) but cache tokens are reported, they fall back to the input
// rate rather than being billed at zero.
func TestPricing_Cost_CacheRateFallback(t *testing.T) {
	p := Pricing{InputPer1M: 2, OutputPer1M: 8} // no cache rates set
	u := &Usage{
		PromptTokens:         1000,
		CompletionTokens:     0,
		CacheReadInputTokens: new(int64(400)),
	}
	// uncached 600 @2 + read 400 @ (fallback) 2 = 1000*2 = 2000 / 1e6
	if got := p.Cost(u); !approx(got, 0.002) {
		t.Errorf("Cost = %v, want 0.002 (cache read falls back to input rate)", got)
	}
}
