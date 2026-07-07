package chat

import "github.com/Tangerg/lynx/pkg/ptr"

// Pricing is one band of a chat model's per-token rate card, expressed
// (like the industry convention) as USD per 1,000,000 tokens. A model's
// full rate card is a []Pricing (see [ModelInfo.Pricing]): usually a
// single band, but long-context models carry more — a base band plus
// higher bands that take over once the prompt crosses [Pricing.Threshold]
// (Gemini 2.5 Pro is $1.25/$10 up to 200K input tokens, $2.50/$15 beyond;
// several OpenAI models split short vs long context the same way).
//
// IMPORTANT: a band reprices the WHOLE prompt, not the marginal tokens
// above the threshold — a 250K-token Gemini 2.5 Pro call is billed
// entirely at the >200K rates, not 200K at base + 50K at the tier. See
// [CostOf] for selection across bands; [Pricing.Cost] is one band alone.
//
// It lives in the chat package because input/output/cache token rates are
// a chat-model concept — other modalities price differently (per image,
// per second of audio). The rate table itself (which model costs what) is
// reference data outside this protocol layer (the models module ships one
// sourced from models.dev).
type Pricing struct {
	// Threshold is the prompt input-token count at or above which this
	// band applies. 0 is the base band (always applicable).
	Threshold int64 `json:"threshold,omitempty"`

	// InputPer1M is the rate for uncached prompt (input) tokens.
	InputPer1M float64 `json:"input_per_1m"`

	// OutputPer1M is the rate for completion (output) tokens.
	OutputPer1M float64 `json:"output_per_1m"`

	// CacheReadPer1M is the (discounted) rate for prompt tokens served
	// from the provider's prompt cache. Zero falls back to InputPer1M.
	CacheReadPer1M float64 `json:"cache_read_per_1m,omitempty"`

	// CacheWritePer1M is the (premium) rate for prompt tokens written to
	// the provider's prompt cache. Zero falls back to InputPer1M.
	CacheWritePer1M float64 `json:"cache_write_per_1m,omitempty"`
}

func (p Pricing) IsZero() bool { return p == Pricing{} }

// CostOf computes the USD cost of one call from a [Usage] breakdown,
// across a banded rate card (see [Pricing]). The prompt's input-token size
// selects the band — the highest [Pricing.Threshold] the prompt reaches,
// scanning back to front (bands are ascending by Threshold) — and the
// whole call bills at that band. Returns 0 for an empty card or nil usage.
func CostOf(bands []Pricing, u *Usage) float64 {
	if u == nil || len(bands) == 0 {
		return 0
	}
	band := bands[0]
	for i := len(bands) - 1; i >= 0; i-- {
		if u.PromptTokens >= bands[i].Threshold {
			band = bands[i]
			break
		}
	}
	return band.Cost(u)
}

// Cost computes the USD cost of one call at THIS band's rates (no band
// selection — see [CostOf] for that).
//
// [Usage.PromptTokens] is the FULL prompt count and already includes any
// cache-read / cache-write portions (per the Usage docs), so those are
// billed at their own rates and subtracted from the uncached remainder.
// A nil/zero cache rate falls back to the standard input rate so a model
// that reports cache tokens but lists no cache rate isn't under-billed.
func (p Pricing) Cost(u *Usage) float64 {
	if u == nil || p.IsZero() {
		return 0
	}
	cacheRead := ptr.From(u.CacheReadInputTokens)
	cacheWrite := ptr.From(u.CacheWriteInputTokens)
	uncachedIn := max(u.PromptTokens-cacheRead-cacheWrite, 0)

	readRate := p.CacheReadPer1M
	if readRate == 0 {
		readRate = p.InputPer1M
	}
	writeRate := p.CacheWritePer1M
	if writeRate == 0 {
		writeRate = p.InputPer1M
	}

	total := float64(uncachedIn)*p.InputPer1M +
		float64(u.CompletionTokens)*p.OutputPer1M +
		float64(cacheRead)*readRate +
		float64(cacheWrite)*writeRate
	return total / 1_000_000
}
