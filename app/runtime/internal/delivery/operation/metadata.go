package operation

import (
	"context"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

type requestMetaKey struct{}

// WithRequestMeta carries validated request metadata across the private
// operation boundary.
func WithRequestMeta(ctx context.Context, meta protocol.RequestMeta) context.Context {
	return context.WithValue(ctx, requestMetaKey{}, cloneRequestMeta(meta))
}

// RequestMetaFrom returns a defensive copy of operation request metadata.
func RequestMetaFrom(ctx context.Context) (protocol.RequestMeta, bool) {
	meta, ok := ctx.Value(requestMetaKey{}).(protocol.RequestMeta)
	return cloneRequestMeta(meta), ok
}

// ClientCapabilitiesFrom returns the capabilities declared for this operation.
func ClientCapabilitiesFrom(ctx context.Context) (*protocol.ClientCapabilities, bool) {
	meta, ok := RequestMetaFrom(ctx)
	if !ok || meta.ClientCapabilities == nil {
		return nil, false
	}
	return meta.ClientCapabilities, true
}

func cloneRequestMeta(meta protocol.RequestMeta) protocol.RequestMeta {
	if meta.ClientInfo != nil {
		info := *meta.ClientInfo
		meta.ClientInfo = &info
	}
	if meta.ClientCapabilities != nil {
		capabilities := *meta.ClientCapabilities
		capabilities.InterruptTypes = slices.Clone(capabilities.InterruptTypes)
		capabilities.ExcludedEphemeralEvents = slices.Clone(capabilities.ExcludedEphemeralEvents)
		capabilities.Features = maps.Clone(capabilities.Features)
		meta.ClientCapabilities = &capabilities
	}
	return meta
}
