package mcp

import (
	"context"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MetaFunc produces the _meta map carried on outbound MCP requests
// (CallTool today; extend to other RPCs as the package grows). It is the
// hook through which a caller forwards ambient identifiers (userId,
// traceId, sessionId, …) from the caller-side context to the remote
// server.
//
// A nil MetaFunc, or one that returns an empty map, sends no _meta.
type MetaFunc func(ctx context.Context) sdkmcp.Meta

type metaContextKey struct{}

// WithMeta returns a copy of ctx carrying meta. Empty meta leaves ctx
// unchanged. Pair with [MetaFromContext] to forward per-request
// metadata across the tool subsystem without explicit plumbing.
func WithMeta(ctx context.Context, meta sdkmcp.Meta) context.Context {
	if len(meta) == 0 {
		return ctx
	}
	return context.WithValue(ctx, metaContextKey{}, meta)
}

// MetaFromContext returns metadata stored by [WithMeta], or nil. Its
// signature matches [MetaFunc] so it can be assigned directly:
//
//	opts := mcp.ToolOptions{MetaFunc: mcp.MetaFromContext}
func MetaFromContext(ctx context.Context) sdkmcp.Meta {
	meta, _ := ctx.Value(metaContextKey{}).(sdkmcp.Meta)
	return meta
}
