package coreapi

// ClientCapabilities is what the client tells Runtime at initialize
// time (API.md §6.1). Server MUST NOT emit events / use features the
// client didn't declare.
type ClientCapabilities struct {
	Events   EventCapabilities         `json:"events"`
	Features ClientFeatureCapabilities `json:"features"`
}

// ServerCapabilities is what Runtime advertises in initialize result.
type ServerCapabilities struct {
	Events    EventCapabilities         `json:"events"`
	Features  ServerFeatureCapabilities `json:"features"`
	Providers []string                  `json:"providers"`
	Limits    Limits                    `json:"limits"`
}

// EventCapabilities — names of AG-UI standard events + Lyra CUSTOM
// events the holder supports.
type EventCapabilities struct {
	Standard []string `json:"standard"`
	Custom   []string `json:"custom"`
}

// ClientFeatureCapabilities — feature toggles the client declares.
// Server consults these before emitting events it might fall back on.
type ClientFeatureCapabilities struct {
	Multimodal *bool `json:"multimodal,omitempty"`
	Markdown   *bool `json:"markdown,omitempty"`
}

// ServerFeatureCapabilities — feature toggles Runtime advertises.
// Unset fields default to false on the client side.
type ServerFeatureCapabilities struct {
	Multimodal     bool                `json:"multimodal"`
	Reasoning      bool                `json:"reasoning"`
	Checkpoints    bool                `json:"checkpoints"`
	Interrupts     bool                `json:"interrupts"`
	Background     bool                `json:"background"`
	Subagents      bool                `json:"subagents"`
	Skills         bool                `json:"skills"`
	MCP            bool                `json:"mcp"`
	SessionExport  bool                `json:"sessionExport"`
	Attachments    AttachmentLimits    `json:"attachments"`
}

// AttachmentLimits is the nested feature shape for attachments.
type AttachmentLimits struct {
	Enabled      bool     `json:"enabled"`
	MaxSizeBytes int64    `json:"maxSizeBytes,omitempty"`
	MimeTypes    []string `json:"mimeTypes,omitempty"`
}

// Limits — server-side hard caps surfaced to the client.
type Limits struct {
	MaxMessagesPerSession int `json:"maxMessagesPerSession,omitempty"`
	MaxConcurrentRuns     int `json:"maxConcurrentRuns,omitempty"`
}
