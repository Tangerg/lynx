package protocol

// ClientCapabilities is what the client declares at initialize (API.md §9).
// Server MUST NOT emit events the client can't render, nor produce an
// open interrupt whose kind the client didn't declare in InterruptKinds.
type ClientCapabilities struct {
	// Events is the set of stream event types the client can render
	// (run.* / item.* / state.* / custom names).
	Events []string `json:"events"`
	// Features is free-form client feature declaration.
	Features map[string]any `json:"features,omitempty"`
	// InterruptKinds are the HITL kinds the client can handle
	// ("approval" | "question" | "toolResult"). Anti-deadlock (API.md §6.2).
	InterruptKinds []string `json:"interruptKinds,omitempty"`
	// OptOutNotificationMethods lets the client suppress high-frequency
	// notifications per connection, e.g. ["item.delta"] (API.md §9).
	OptOutNotificationMethods []string `json:"optOutNotificationMethods,omitempty"`
}

// ServerCapabilities is what Runtime advertises in the initialize result
// and the /v2/info sidecar (API.md §9).
type ServerCapabilities struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Events          []string       `json:"events"`
	Features        ServerFeatures `json:"features"`
	Providers       []string       `json:"providers"`
	Limits          RuntimeLimits  `json:"limits"`
}

// ServerFeatures are the feature toggles Runtime advertises. Unset
// (false) means the corresponding methods/events are unavailable.
type ServerFeatures struct {
	Reasoning     bool             `json:"reasoning"`
	MCP           bool             `json:"mcp"`
	Multimodal    bool             `json:"multimodal"`
	Checkpoints   bool             `json:"checkpoints"`
	Background    bool             `json:"background"`
	Subagents     bool             `json:"subagents"`
	Skills        bool             `json:"skills"`
	SessionExport bool             `json:"sessionExport"`
	Memory        bool             `json:"memory"`
	Relocate      bool             `json:"relocate"`
	ClientTools   bool             `json:"clientTools"`
	Attachments   AttachmentLimits `json:"attachments"`
}

// AttachmentLimits is the nested feature shape for attachments.
type AttachmentLimits struct {
	Enabled      bool     `json:"enabled"`
	MaxSizeBytes int64    `json:"maxSizeBytes,omitempty"`
	MimeTypes    []string `json:"mimeTypes,omitempty"`
}

// RuntimeLimits — server-side hard caps surfaced to the client.
type RuntimeLimits struct {
	MaxConcurrentRuns  int `json:"maxConcurrentRuns,omitempty"`
	MaxItemsPerSession int `json:"maxItemsPerSession,omitempty"`
}
