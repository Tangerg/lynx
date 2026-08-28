package runmaintenance

// compactionDefaults govern the auto-compact trigger. Tunable via
// [CompactionConfig]. Two independent triggers, OR-composed: a raw
// message count and an estimated token footprint. The token trigger
// catches a conversation whose few messages carry large tool outputs (a
// single file read can outweigh twenty short Runs) — which a message
// count alone misses. The defaults aim at "comfortably fits in
// 128k-context models".
const (
	percentageScale = 100

	defaultCompactMaxMessages = 24 // message count trigger threshold
	defaultCompactKeepRecent  = 6  // raw messages to preserve verbatim

	// defaultCompactMaxTokens is the estimated-token-footprint trigger used
	// ONLY when the model's real context window is unknown (catalog miss). When
	// the window IS known the trigger is window-relative instead — see
	// [CompactionConfig.ContextWindow] / [windowTriggerPct], capped by
	// [CompactionConfig.MaxInputTokens] when the provider publishes one.
	defaultCompactMaxTokens = 100_000

	// windowTriggerPct is the share of the model's context window at which an
	// estimated footprint triggers compaction — leaving headroom for the
	// summary output + the next Run. A fixed number is wrong across the 32k…1M
	// window range; a relative trigger tracks the actual model's context
	// window rather than a fixed number that's wrong at either extreme.
	windowTriggerPct = 80
)

// CompactionConfig tunes the auto-compaction heuristic.
//
// A sweep triggers when EITHER bound is breached: MaxMessages (raw
// message count) or MaxTokens (estimated token footprint). On a sweep the
// oldest (len - KeepRecent) messages are replaced by a single system
// message carrying an LLM-generated summary.
//
// Zero values fall back to the package defaults.
type CompactionConfig struct {
	MaxMessages int // default: defaultCompactMaxMessages
	MaxTokens   int // explicit token-footprint trigger; still capped by the provider's hard input limit
	KeepRecent  int // default: defaultCompactKeepRecent
	// ContextWindow is the model's context window in tokens (from the catalog).
	// When > 0 and MaxTokens is unset, the token trigger becomes
	// ContextWindow*windowTriggerPct% — relative to the real model instead of a
	// fixed number. 0 (catalog miss) falls back to defaultCompactMaxTokens.
	ContextWindow int
	// MaxInputTokens is the provider's hard prompt limit. When positive it caps
	// every derived or explicit trigger; a maintenance setting cannot authorize
	// a request the selected model must reject.
	MaxInputTokens int
}
