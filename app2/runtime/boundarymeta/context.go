// Package boundarymeta carries transport-neutral metadata from delivery into
// application use cases. It deliberately knows neither HTTP nor JSON-RPC.
package boundarymeta

import (
	"context"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type requestMetaKey struct{}
type afterEventIDKey struct{}

func WithRequestMeta(ctx context.Context, meta protocol.RequestMeta) context.Context {
	return context.WithValue(ctx, requestMetaKey{}, cloneRequestMeta(meta))
}

func RequestMetaFrom(ctx context.Context) (protocol.RequestMeta, bool) {
	meta, ok := ctx.Value(requestMetaKey{}).(protocol.RequestMeta)
	return cloneRequestMeta(meta), ok
}

func ClientCapabilitiesFrom(ctx context.Context) (*protocol.ClientCapabilities, bool) {
	meta, ok := RequestMetaFrom(ctx)
	if !ok || meta.ClientCapabilities == nil {
		return nil, false
	}
	return meta.ClientCapabilities, true
}

func WithAfterEventID(ctx context.Context, eventID string) context.Context {
	if eventID == "" {
		return ctx
	}
	return context.WithValue(ctx, afterEventIDKey{}, eventID)
}

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
