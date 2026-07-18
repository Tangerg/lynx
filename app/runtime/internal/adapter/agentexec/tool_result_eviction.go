package agentexec

import "context"

// toolResultOffloader is the narrow write capability the observer needs to evict
// an oversized tool result: Offload stores the full body and returns the id that
// the placeholder (and later the transcript presenter and the read_tool_result
// tool) recover it by. nil disables eviction.
type toolResultOffloader interface {
	Offload(ctx context.Context, sessionID, toolName, body string) (id string, err error)
}

// toolResultPreviewBytes bounds the head+tail preview left inline once a body is
// offloaded, so the evicted result is at most this many bytes (plus the marker)
// regardless of the original size. Capped to the eviction threshold so the
// placeholder is always smaller than the body that tripped it.
const toolResultPreviewBytes = 2000
