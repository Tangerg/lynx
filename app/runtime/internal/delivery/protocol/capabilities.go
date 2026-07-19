package protocol

// ClientCapabilities is what the client declares in request metadata (API.md §9).
// Server MUST NOT emit events the client can't render, nor produce an
// open interrupt whose type the client didn't declare in InterruptTypes.
type ClientCapabilities struct {
	// Events is the set of stream event types the client can render
	// (run.* / item.* / state.* / custom names).
	Events   []StreamEventType            `json:"events"`
	Features map[string]FeaturePreference `json:"features,omitempty"`
	// InterruptTypes are the HITL interrupt types the client can handle
	// (see InterruptType). Anti-deadlock (API.md §6.2).
	InterruptTypes []InterruptType `json:"interruptTypes,omitempty"`
	// ExcludedEvents lets the client suppress high-frequency
	// notifications per connection, by event type, e.g. [StreamItemDelta]
	// (API.md §9). The values are StreamEventTypes (the wire field keeps its
	// historical "...Methods" name).
	ExcludedEvents []StreamEventType `json:"excludedEvents,omitempty"`
}

type FeaturePreference struct {
	Enabled bool `json:"enabled"`
}

// ServerCapabilities is what Runtime advertises in runtime.discover
// and the /v2/info sidecar (API.md §9).
type ServerCapabilities struct {
	Events           []StreamEventType            `json:"events"`
	StreamingMethods []string                     `json:"streamingMethods"`
	Features         map[string]FeatureCapability `json:"features"`
	Limits           RuntimeLimits                `json:"limits"`
}

type Stability string

const (
	StabilityStable       Stability = "stable"
	StabilityExperimental Stability = "experimental"
)

type FeatureCapability struct {
	Enabled   bool      `json:"enabled"`
	Stability Stability `json:"stability"`
}

// RuntimeLimits — server-side hard caps surfaced to the client.
type RuntimeLimits struct {
	MaxConcurrentRuns int `json:"maxConcurrentRuns,omitempty"`
}
