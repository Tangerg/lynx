package protocol

// ClientCapabilities is what the client declares at initialize (API.md §9).
// Server MUST NOT emit events the client can't render, nor produce an
// open interrupt whose type the client didn't declare in InterruptTypes.
type ClientCapabilities struct {
	// Events is the set of stream event types the client can render
	// (run.* / item.* / state.* / custom names).
	Events []string `json:"events"`
	// Features is free-form client feature declaration.
	Features map[string]any `json:"features,omitempty"`
	// InterruptTypes are the HITL interrupt types the client can handle
	// ("approval" | "question" | "toolResult"). Anti-deadlock (API.md §6.2).
	InterruptTypes []string `json:"interruptTypes,omitempty"`
	// OptOutNotificationMethods lets the client suppress high-frequency
	// notifications per connection, e.g. ["item.delta"] (API.md §9).
	OptOutNotificationMethods []string `json:"optOutNotificationMethods,omitempty"`
}

// ServerCapabilities is what Runtime advertises in the initialize result
// and the /v2/info sidecar (API.md §9).
type ServerCapabilities struct {
	ProtocolVersion  string                 `json:"protocolVersion"`
	Events           []string               `json:"events"`
	StreamingMethods []string               `json:"streamingMethods"`
	Features         map[string]FeatureFlag `json:"features"`
	Providers        []string               `json:"providers"`
	Limits           RuntimeLimits          `json:"limits"`
}

// FeatureFlag is one entry in the open ServerCapabilities.features map
// (API.md §9): `boolean | { enabled: boolean; … }`. The map is OPEN
// (symmetric with ClientCapabilities.features) so advertising a new
// capability is adding a key — old clients ignore unknown keys, the
// contract isn't bumped. Most known keys are plain bools; `attachments`
// is the object form (AttachmentLimits).
type FeatureFlag = any

// AttachmentLimits is the object-form FeatureFlag for `attachments`.
type AttachmentLimits struct {
	Enabled      bool     `json:"enabled"`
	MaxSizeBytes int64    `json:"maxSizeBytes,omitempty"`
	MimeTypes    []string `json:"mimeTypes,omitempty"`
}

// RuntimeLimits — server-side hard caps surfaced to the client.
type RuntimeLimits struct {
	MaxConcurrentRuns int `json:"maxConcurrentRuns,omitempty"`
}
