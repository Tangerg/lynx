// Package accounting owns the durable attribution facts used by usage reports.
package accounting

import "time"

type ModelUsage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	CostUSD          *float64
}

type Usage struct {
	ModelUsage
	ByModel map[string]ModelUsage
}

type RunRecord struct {
	SessionID string
	Provider string
	Model string
	Usage *Usage
	FinishedAt time.Time
}
