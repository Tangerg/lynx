package operation

import (
	"context"
	"maps"
	"slices"

	"github.com/Tangerg/scope/app/runtime/protocol"
)

type requestMetaKey struct{}
type afterEventIDKey struct{}

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

func withAfterEventID(ctx context.Context, eventID string) context.Context {
	if eventID == "" {
		return ctx
	}
	return context.WithValue(ctx, afterEventIDKey{}, eventID)
}

// AfterEventIDFrom returns the stream cursor attached at the operation boundary.
// Application-facing implementations read this binding-neutral value rather than
// depending on an HTTP header or transport context key.
func AfterEventIDFrom(ctx context.Context) string {
	eventID, _ := ctx.Value(afterEventIDKey{}).(string)
	return eventID
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
