package agentexec

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
)

// toolResultOffloader is the narrow write capability the observer needs to evict
// an oversized tool result: Offload stages the full body and returns the typed
// identity carried by the preview, transcript event, and read_tool_result call.
// nil disables eviction.
type toolResultOffloader interface {
	Offload(ctx context.Context, sessionID, toolName, body string) (offload.ID, error)
	Discard(ctx context.Context, sessionID string, ref offload.Ref) error
}

// toolResultPreviewBytes bounds the head+tail preview left inline once a body is
// offloaded, so the candidate preview keeps at most this many body bytes plus
// the retrieval marker. The observer rejects the candidate if that fixed marker
// makes it no smaller than the original body.
const toolResultPreviewBytes = 2000
