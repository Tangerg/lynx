package mcp

import (
	"context"
	"maps"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// RequestMetaFunc resolves per-call MCP metadata from the context rather than
// from a fixed value, so a caller can forward request-scoped identity such as a
// trace or tenant without rebuilding the tool for every call.
type RequestMetaFunc func(ctx context.Context) sdkmcp.Meta

type requestMetaContextKey struct{}

// WithRequestMeta stores a defensive snapshot because request metadata may be
// read after the caller reuses or mutates its original map.
func WithRequestMeta(ctx context.Context, meta sdkmcp.Meta) context.Context {
	if len(meta) == 0 {
		return ctx
	}
	return context.WithValue(ctx, requestMetaContextKey{}, maps.Clone(meta))
}

// RequestMetaFromContext returns a shallow copy of metadata stored by
// [WithRequestMeta], or nil. Its signature matches [RequestMetaFunc]:
//
//	config := mcp.ToolDiscoveryConfig{RequestMeta: mcp.RequestMetaFromContext}
func RequestMetaFromContext(ctx context.Context) sdkmcp.Meta {
	meta, _ := ctx.Value(requestMetaContextKey{}).(sdkmcp.Meta)
	return maps.Clone(meta)
}
