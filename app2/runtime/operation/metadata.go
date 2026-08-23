package operation

import (
	"context"

	"github.com/Tangerg/lynx/app2/runtime/boundarymeta"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

// WithRequestMeta carries validated request metadata across the private
// operation boundary.
func WithRequestMeta(ctx context.Context, meta protocol.RequestMeta) context.Context {
	return boundarymeta.WithRequestMeta(ctx, meta)
}

// RequestMetaFrom returns a defensive copy of operation request metadata.
func RequestMetaFrom(ctx context.Context) (protocol.RequestMeta, bool) {
	return boundarymeta.RequestMetaFrom(ctx)
}

// ClientCapabilitiesFrom returns the capabilities declared for this operation.
func ClientCapabilitiesFrom(ctx context.Context) (*protocol.ClientCapabilities, bool) {
	return boundarymeta.ClientCapabilitiesFrom(ctx)
}

func withAfterEventID(ctx context.Context, eventID string) context.Context {
	return boundarymeta.WithAfterEventID(ctx, eventID)
}

// AfterEventIDFrom returns the stream cursor attached at the operation boundary.
// Application-facing implementations read this binding-neutral value rather than
// depending on an HTTP header or transport context key.
func AfterEventIDFrom(ctx context.Context) string {
	return boundarymeta.AfterEventIDFrom(ctx)
}
