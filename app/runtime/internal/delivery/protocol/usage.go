package protocol

import "context"

// UsageReports is the usage.* method group (API.md §7.7) — read-only spend
// reporting aggregated from the durable run history. Named UsageReports (not
// Usage) to avoid clashing with the [Usage] token-usage type.
type UsageReports interface {
	// SessionUsage returns one session's cumulative token usage + cost, summed
	// over its finished runs (usage.session).
	SessionUsage(ctx context.Context, sessionID string) (*Usage, error)
	// UsageSummary returns a cross-session spend report — totals plus
	// per-provider / per-model / per-day breakdowns (usage.summary).
	UsageSummary(ctx context.Context, in UsageSummaryRequest) (*UsageSummary, error)
}

// UsageSummaryRequest — usage.summary body (API.md §7.7).
type UsageSummaryRequest struct {
	// SinceDays limits the report to runs finished within the last N days;
	// 0 (the zero value) means all time.
	SinceDays int `json:"sinceDays,omitempty"`
}

// UsageBucket is one grouped slice of usage — a provider id, a "provider/model"
// pair, or a day (YYYY-MM-DD) — carrying its rolled-up tokens + cost and the
// number of runs that contributed.
type UsageBucket struct {
	Key string `json:"key"`
	ModelUsage
	Runs int `json:"runs,omitempty"`
}

// UsageSummary is the cross-session spend report (usage.summary). Every bucket
// sums whole-run totals, so the breakdowns reconcile with Total. Attribution is
// at run granularity: a run's spend lands under the model/provider the run ran
// against — a run's incidental utility-model work (compaction / titling) is
// folded into that headline model, not split out.
type UsageSummary struct {
	Total      ModelUsage    `json:"total"`
	ByProvider []UsageBucket `json:"byProvider,omitempty"`
	ByModel    []UsageBucket `json:"byModel,omitempty"`
	ByDay      []UsageBucket `json:"byDay,omitempty"`
	// Sessions is the number of user-facing sessions with any recorded spend;
	// Runs is the number of finished runs counted.
	Sessions int `json:"sessions,omitempty"`
	Runs     int `json:"runs,omitempty"`
}
