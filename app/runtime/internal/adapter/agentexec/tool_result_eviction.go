package agentexec

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
)

// toolResultOffloader is the narrow write capability the observer needs to
// persist a body after its candidate preview has proven worth evicting. nil
// disables eviction.
type toolResultOffloader interface {
	Stage(ctx context.Context, stage offload.ToolResultStage) error
}

// toolResultPreviewBytes bounds the head+tail preview left inline once a body is
// offloaded, so the candidate preview keeps at most this many body bytes plus
// the retrieval marker. The observer rejects the candidate if that fixed marker
// makes it no smaller than the original body.
const toolResultPreviewBytes = 2000
