package catalog

// Pricing is one rate-card band in USD per one million tokens. Threshold is
// the input-token count at which the band reprices the whole call.
type Pricing struct {
	Threshold       int64   `json:"threshold,omitempty"`
	InputPer1M      float64 `json:"input_per_1m"`
	OutputPer1M     float64 `json:"output_per_1m"`
	CacheReadPer1M  float64 `json:"cache_read_per_1m,omitempty"`
	CacheWritePer1M float64 `json:"cache_write_per_1m,omitempty"`
}

func (p Pricing) IsZero() bool { return p == Pricing{} }

// Usage is the provider-neutral token breakdown required for pricing.
type Usage struct {
	InputTokens           int64
	OutputTokens          int64
	CacheReadInputTokens  int64
	CacheWriteInputTokens int64
}

// CostOf selects the highest applicable pricing band and computes USD cost.
func CostOf(bands []Pricing, usage Usage) float64 {
	if len(bands) == 0 {
		return 0
	}
	band := bands[0]
	for i := len(bands) - 1; i >= 0; i-- {
		if usage.InputTokens >= bands[i].Threshold {
			band = bands[i]
			break
		}
	}
	return band.Cost(usage)
}

// Cost computes USD cost using this pricing band.
func (p Pricing) Cost(usage Usage) float64 {
	if p.IsZero() {
		return 0
	}
	cacheRead := max(usage.CacheReadInputTokens, 0)
	cacheWrite := max(usage.CacheWriteInputTokens, 0)
	uncachedInput := max(usage.InputTokens-cacheRead-cacheWrite, 0)

	readRate := p.CacheReadPer1M
	if readRate == 0 {
		readRate = p.InputPer1M
	}
	writeRate := p.CacheWritePer1M
	if writeRate == 0 {
		writeRate = p.InputPer1M
	}

	total := float64(uncachedInput)*p.InputPer1M +
		float64(usage.OutputTokens)*p.OutputPer1M +
		float64(cacheRead)*readRate +
		float64(cacheWrite)*writeRate
	return total / 1_000_000
}
