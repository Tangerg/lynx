package protocol

// SessionUsageRequest identifies the session whose aggregate usage is read.
type SessionUsageRequest struct {
	SessionID string `json:"sessionId"`
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
// against. Request-detached utility-model maintenance is not part of Run usage.
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
