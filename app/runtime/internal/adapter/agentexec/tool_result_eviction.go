package agentexec

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turnctx"
)

// toolResultOffloader is the narrow write capability the eviction middleware
// needs from the offloaded-tool-result store (consumer-side interface). Offload
// stores body and returns the id a later read_tool_result call retrieves it by.
type toolResultOffloader interface {
	Offload(ctx context.Context, sessionID, toolName, body string) (id string, err error)
}

// toolResultPreviewBytes bounds the head+tail preview kept inline after a body
// is offloaded. The evicted message is at most this many bytes (plus the
// pointer marker) regardless of the original size, so one oversized result
// stops re-inflating every subsequent LLM request. Capped to the threshold so
// the placeholder is always smaller than the body that tripped it.
const toolResultPreviewBytes = 2000

// toolResultEvictionMiddleware offloads oversized tool-result bodies to the blob
// store and substitutes a head+tail placeholder pointing at read_tool_result,
// so a single huge output leaves the LLM context yet stays retrievable. It is
// registered OUTER to the tool observer, so the observation/UI event still sees
// the full body (observedTool.finish fires before this wrapper returns) and only
// the value flowing on to history is trimmed.
//
// The session id is read per call from the turn context ([turnctx.TurnSession]),
// the same source the read-back tool uses — so a root turn and a delegated
// subtask each store and retrieve under their own session automatically, with no
// session id threaded through construction. A zero threshold or nil store
// disables it (WrapTool returns the tool unchanged); an empty session at call
// time leaves the body intact (nothing to scope the blob under).
type toolResultEvictionMiddleware struct {
	store     toolResultOffloader
	threshold int
}

// newToolResultEviction returns the eviction middleware, or nil when eviction is
// disabled (no store, or a non-positive threshold). Shared by the root turn and
// delegated subtasks so both offload oversized results the same way.
func newToolResultEviction(store toolResultOffloader, threshold int) core.Extension {
	if store == nil || threshold <= 0 {
		return nil
	}
	return &toolResultEvictionMiddleware{store: store, threshold: threshold}
}

// Name implements [core.Extension]. Process-scoped, so a constant name is fine
// (it may collide with engine-scope names).
func (*toolResultEvictionMiddleware) Name() string { return "tool-result-eviction" }

// WrapTool decorates tool with offloading unless the tool is the read-back tool
// itself (evicting its output would loop) or the middleware is disabled.
func (m *toolResultEvictionMiddleware) WrapTool(_ core.ProcessView, _ core.Action, tool tools.Tool) tools.Tool {
	if tool.Definition().Name == toolport.ToolNameReadToolResult {
		return tool
	}
	return &evictingTool{inner: tool, mw: m}
}

// evictingTool is the per-call wrapper. It forwards the wrapped tool's optional
// scheduling / continuation / mutation capabilities structurally (dropping them
// would silently serialize concurrent tools and break file-aware outer
// middleware — the same forwarding [observedTool] does) and offloads an
// oversized successful result before returning.
type evictingTool struct {
	inner tools.Tool
	mw    *toolResultEvictionMiddleware
}

var _ tools.FileMutationReporter = (*evictingTool)(nil)

func (t *evictingTool) Definition() chat.ToolDefinition { return t.inner.Definition() }

func (t *evictingTool) ReturnsDirect() bool {
	if direct, ok := t.inner.(interface{ ReturnsDirect() bool }); ok {
		return direct.ReturnsDirect()
	}
	return false
}

func (t *evictingTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	if capability, ok := t.inner.(interface {
		ConcurrencyKey(string) (string, bool)
	}); ok {
		return capability.ConcurrencyKey(arguments)
	}
	return "", false
}

func (t *evictingTool) MutationPaths(arguments string) ([]string, error) {
	if reporter, ok := t.inner.(tools.FileMutationReporter); ok {
		return reporter.MutationPaths(arguments)
	}
	return nil, nil
}

func (t *evictingTool) Call(ctx context.Context, arguments string) (string, error) {
	output, err := t.inner.Call(ctx, arguments)
	// Only offload a successful, oversized body. A failed call's (short) error
	// text is left intact; the model needs to read it to recover.
	if err != nil || t.mw.threshold <= 0 || len(output) <= t.mw.threshold {
		return output, err
	}
	sessionID := turnctx.TurnSession(ctx)
	if sessionID == "" {
		// No session to scope the blob under (or retrieve it later) — leave the
		// body intact rather than offload something unreachable.
		return output, nil
	}
	id, offloadErr := t.mw.store.Offload(ctx, sessionID, t.inner.Definition().Name, output)
	if offloadErr != nil {
		// Best-effort: a failed offload degrades to the full body (context bloat,
		// not a broken turn) rather than failing an otherwise-successful call.
		return output, nil
	}
	return offloadPlaceholder(output, id, t.mw.threshold), nil
}

// offloadPlaceholder replaces an offloaded body with a head+tail preview and a
// pointer telling the model how to retrieve the full content. preview is capped
// to the threshold so the placeholder is always smaller than the body that
// tripped eviction. Head/tail cuts snap to rune boundaries.
func offloadPlaceholder(body, id string, threshold int) string {
	preview := min(toolResultPreviewBytes, threshold)
	head := preview * 3 / 4
	tailStart := len(body) - preview/4
	for head > 0 && !utf8.RuneStart(body[head]) {
		head--
	}
	for tailStart < len(body) && !utf8.RuneStart(body[tailStart]) {
		tailStart++
	}
	marker := fmt.Sprintf(
		"\n\n…[%d bytes offloaded to keep context small. Retrieve the full output with the %s tool: {\"id\":\"%s\"} — it pages via offset/limit.]…\n\n",
		len(body), toolport.ToolNameReadToolResult, id,
	)
	return body[:head] + marker + body[tailStart:]
}
