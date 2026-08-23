package protocol

// RequestMeta carries binding-neutral request metadata. HTTP projects it from
// params._meta; embedded callers provide the same value through call options.
type RequestMeta struct {
	ProtocolVersion    string              `json:"protocolVersion,omitempty"`
	ClientInfo         *ClientInfo         `json:"clientInfo,omitempty"`
	ClientCapabilities *ClientCapabilities `json:"clientCapabilities,omitempty"`
}
