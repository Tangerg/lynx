package chat

// Pricing is a chat model's per-token rate card, expressed (like the
// industry convention and the catwalk catalog) as USD per 1,000,000
// tokens. It lives in the chat package because input/output/cache token
// rates are a chat-model concept — other modalities price differently
// (per image, per second of audio), so this isn't a generic model type.
//
// The zero value means "unknown" — treat a zero Pricing as "no cost
// available", not "free". Surface it via [ModelMetadata.Pricing] so cost
// can be attributed without the consumer hard-coding a rate table. The
// rate table itself (which model costs what) is reference data that
// tracks vendor pricing and lives outside this protocol layer (see
// github.com/Tangerg/lynx/models/pricing, modeled on charm.land/catwalk).
type Pricing struct {
	// InputPer1M is the rate for uncached prompt (input) tokens.
	InputPer1M float64 `json:"input_per_1m"`

	// OutputPer1M is the rate for completion (output) tokens.
	OutputPer1M float64 `json:"output_per_1m"`

	// CacheReadPer1M is the (discounted) rate for prompt tokens served
	// from the provider's prompt cache. Zero falls back to InputPer1M.
	CacheReadPer1M float64 `json:"cache_read_per_1m"`

	// CacheWritePer1M is the (premium) rate for prompt tokens written to
	// the provider's prompt cache. Zero falls back to InputPer1M.
	CacheWritePer1M float64 `json:"cache_write_per_1m"`
}

// IsZero reports whether the rate card is unset (all rates zero) — i.e.
// pricing is unknown for this model.
func (p Pricing) IsZero() bool {
	return p == Pricing{}
}

// Cost computes the USD cost of one call from a [Usage] breakdown.
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
	cacheRead := derefInt64(u.CacheReadInputTokens)
	cacheWrite := derefInt64(u.CacheWriteInputTokens)
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

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
