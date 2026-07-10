package protocol

// ClientCapabilities is what the client declares in request metadata (API.md §9).
// Server MUST NOT emit events the client can't render, nor produce an
// open interrupt whose type the client didn't declare in InterruptTypes.
type ClientCapabilities struct {
	// Events is the set of stream event types the client can render
	// (run.* / item.* / state.* / custom names).
	Events []StreamEventType `json:"events"`
	// Features is free-form client feature declaration.
	Features map[string]any `json:"features,omitempty"`
	// InterruptTypes are the HITL interrupt types the client can handle
	// (see InterruptType). Anti-deadlock (API.md §6.2).
	InterruptTypes []InterruptType `json:"interruptTypes,omitempty"`
	// OptOutNotificationMethods lets the client suppress high-frequency
	// notifications per connection, by event type, e.g. [StreamItemDelta]
	// (API.md §9). The values are StreamEventTypes (the wire field keeps its
	// historical "...Methods" name).
	OptOutNotificationMethods []StreamEventType `json:"optOutNotificationMethods,omitempty"`
}

// ServerCapabilities is what Runtime advertises in runtime.discover
// and the /v2/info sidecar (API.md §9).
type ServerCapabilities struct {
	ProtocolVersion  string                 `json:"protocolVersion"`
	Events           []StreamEventType      `json:"events"`
	StreamingMethods []string               `json:"streamingMethods"`
	Features         map[string]FeatureFlag `json:"features"`
	Providers        []string               `json:"providers"`
	Limits           RuntimeLimits          `json:"limits"`
}

// FeatureFlag is one entry in the open ServerCapabilities.features map
// (API.md §9): `boolean | { enabled: boolean; … }`. The map is OPEN
// (symmetric with ClientCapabilities.features) so advertising a new
// capability is adding a key — old clients ignore unknown keys, the
// contract isn't bumped. Known keys are plain bools.
type FeatureFlag = any

// RuntimeLimits — server-side hard caps surfaced to the client.
type RuntimeLimits struct {
	MaxConcurrentRuns int `json:"maxConcurrentRuns,omitempty"`
}
