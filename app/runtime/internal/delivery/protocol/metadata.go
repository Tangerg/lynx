package protocol

import (
	"context"
	"maps"
	"slices"
)

// RequestMeta carries protocol metadata embedded under params._meta.
// Dispatch strips it before typed business-param decoding and stores it on the
// context for runtime methods that need client-scoped capabilities.
type RequestMeta struct {
	ProtocolVersion    string              `json:"protocolVersion,omitempty"`
	ClientInfo         *ClientInfo         `json:"clientInfo,omitempty"`
	ClientCapabilities *ClientCapabilities `json:"clientCapabilities,omitempty"`
}

type requestMetaKey struct{}

// WithRequestMeta returns ctx carrying request metadata.
func WithRequestMeta(ctx context.Context, meta RequestMeta) context.Context {
	return context.WithValue(ctx, requestMetaKey{}, cloneRequestMeta(meta))
}

// RequestMetaFrom reads request metadata from ctx.
func RequestMetaFrom(ctx context.Context) (RequestMeta, bool) {
	meta, ok := ctx.Value(requestMetaKey{}).(RequestMeta)
	return cloneRequestMeta(meta), ok
}

// ClientCapabilitiesFrom returns the client capabilities carried by the
// current request, if any.
func ClientCapabilitiesFrom(ctx context.Context) (*ClientCapabilities, bool) {
	meta, ok := RequestMetaFrom(ctx)
	if !ok || meta.ClientCapabilities == nil {
		return nil, false
	}
	return meta.ClientCapabilities, true
}

func cloneRequestMeta(meta RequestMeta) RequestMeta {
	if meta.ClientInfo != nil {
		info := *meta.ClientInfo
		meta.ClientInfo = &info
	}
	if meta.ClientCapabilities != nil {
		caps := *meta.ClientCapabilities
		caps.Events = slices.Clone(caps.Events)
		caps.InterruptTypes = slices.Clone(caps.InterruptTypes)
		caps.ExcludedEvents = slices.Clone(caps.ExcludedEvents)
		caps.Features = maps.Clone(caps.Features)
		meta.ClientCapabilities = &caps
	}
	return meta
}
