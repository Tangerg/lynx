package mcp

import (
	"context"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// sessionKey is the unexported context-key type for the per-call MCP
// server session. The session is stamped onto the context by lynx's
// tool dispatcher ([serverHandler]) and retrieved by tool authors via
// [ServerSessionFromContext].
type sessionKey struct{}

// progressTokenKey carries the progress token sent by the client in
// the originating `tools/call` request. It is opaque (`any`) because
// the MCP spec lets clients pick string or number tokens.
type progressTokenKey struct{}

// withProgressToken stamps the originating call's progress token onto
// ctx so [ReportProgress] can echo it back when emitting progress
// notifications. Calls without a progress token leave ctx unchanged.
func withProgressToken(ctx context.Context, req *sdkmcp.CallToolRequest) context.Context {
	if req == nil || req.Params == nil {
		return ctx
	}
	tok := req.Params.GetProgressToken()
	if tok == nil {
		return ctx
	}
	return context.WithValue(ctx, progressTokenKey{}, tok)
}

// progressTokenFromContext returns the progress token associated with
// the current MCP tool invocation, or nil when the client did not
// request progress reporting (or when ctx was not produced by the
// lynx dispatcher).
func progressTokenFromContext(ctx context.Context) any {
	if ctx == nil {
		return nil
	}
	return ctx.Value(progressTokenKey{})
}

// WithServerSession returns a copy of ctx carrying the MCP server
// session. The lynx tool dispatcher calls this before invoking a
// [chat.Tool] so reverse-capability helpers ([ReportProgress],
// [ElicitFromClient], [LogToClient]) can recover the session from
// context.
//
// Application code rarely needs to call this directly — it is exposed
// for tests and for callers building bespoke dispatch loops.
func WithServerSession(ctx context.Context, session *sdkmcp.ServerSession) context.Context {
	if session == nil {
		return ctx
	}
	return context.WithValue(ctx, sessionKey{}, session)
}

// ServerSessionFromContext returns the MCP server session that the
// dispatcher attached to ctx, or nil when called outside an MCP tool
// invocation.
//
// Tool authors generally prefer the higher-level helpers
// ([ReportProgress], [ElicitFromClient], [LogToClient]) over reading
// the session directly; this accessor exists for callers that need
// the full [*sdkmcp.ServerSession] surface (ListRoots, sampling
// initiation, ...).
//
// Example — guard for non-MCP invocations:
//
//	if ss := mcp.ServerSessionFromContext(ctx); ss != nil {
//	    _ = ss.Ping(ctx, nil)
//	}
func ServerSessionFromContext(ctx context.Context) *sdkmcp.ServerSession {
	if ctx == nil {
		return nil
	}
	ss, _ := ctx.Value(sessionKey{}).(*sdkmcp.ServerSession)
	return ss
}
