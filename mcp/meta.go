package mcp

import (
	"context"
	"maps"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// RequestMetaFunc produces the _meta map carried on outbound MCP tool calls.
// It is the hook through which a caller forwards ambient identifiers from the
// caller-side context to the remote server.
//
// A nil RequestMetaFunc, or one that returns an empty map, sends no _meta.
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
