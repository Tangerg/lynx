package mcp

import sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

var _ ConcurrencyFunc = AnnotatedReadOnlyConcurrency

// AnnotatedReadOnlyConcurrency opts explicitly read-only MCP tools into
// conflict-free concurrent execution. Missing, false, or contradictory
// annotations remain exclusive.
//
// This is only scheduling advice: it neither authorizes a call nor bypasses a
// caller's approval policy. MCP annotations are untrusted hints, so callers
// should use this policy only for servers whose descriptors they are willing
// to trust for execution ordering.
func AnnotatedReadOnlyConcurrency(_ string, tool *sdkmcp.Tool, _ string) (key string, concurrent bool) {
	if tool == nil || tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
		return "", false
	}
	if destructive := tool.Annotations.DestructiveHint; destructive != nil && *destructive {
		return "", false
	}
	return "", true
}
