package chat

// Pricing is a chat model's per-token rate card, expressed (like the
// industry convention and the catwalk catalog) as USD per 1,000,000
// tokens. It lives in the chat package because input/output/cache token
// rates are a chat-model concept — other modalities price differently
// (per image, per second of audio), so this isn't a generic model type.
//
// The zero value means "unknown" — treat a zero Pricing as "no cost
// available", not "free". Surface it via [ModelInfo.Pricing] so cost can
// be attributed without the consumer hard-coding a rate table. The rate
// table itself (which model costs what) is reference data that tracks
// vendor pricing and lives outside this protocol layer (see
// github.com/Tangerg/lynx/models/catalog, modeled on charm.land/catwalk).
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

// Reasoning describes a chat model's extended-thinking (reasoning)
// support. Like [Pricing], it's a value type with an [IsZero] check and
// nests inside [ModelInfo].
//
// Supported is the authoritative "can this model reason" bit. Levels and
// DefaultLevel are populated only when effort is level-controlled (e.g.
// OpenAI's "low"/"medium"/"high", Gemini's tiers); a model that reasons
// via a token budget (Anthropic) reports Supported with no Levels.
type Reasoning struct {
	// Supported reports whether the model can reason / think at all.
	Supported bool `json:"supported,omitempty"`

	// Levels are the discrete effort levels the model accepts, in
	// increasing order (e.g. "low", "medium", "high"). Nil when effort
	// isn't level-controlled.
	Levels []string `json:"levels,omitempty"`

	// DefaultLevel is the effort used when the caller doesn't pick one.
	// Empty when there are no Levels.
	DefaultLevel string `json:"default_level,omitempty"`
}

// IsZero reports whether reasoning is unset — i.e. the model can't reason.
func (r Reasoning) IsZero() bool {
	return !r.Supported && len(r.Levels) == 0 && r.DefaultLevel == ""
}
