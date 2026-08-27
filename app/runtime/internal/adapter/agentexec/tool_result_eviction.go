package agentexec

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/domain/toolresult"
)

// toolResultOffloader is the narrow write capability the observer needs to
// persist a body after its candidate preview has proven worth evicting. nil
// disables eviction.
type toolResultOffloader interface {
	Stage(ctx context.Context, stage toolresult.Stage) error
}

// toolResultPreviewBytes bounds the head+tail preview left inline once a body is
// offloaded, so the candidate preview keeps at most this many body bytes plus
// the retrieval marker. The observer rejects the candidate if that fixed marker
// makes it no smaller than the original body.
const toolResultPreviewBytes = 2000

func evictToolResult(
	ctx context.Context,
	store toolResultOffloader,
	threshold int,
	readToolName string,
	sessionID string,
	toolName string,
	output string,
) (string, *toolresult.Ref) {
	if store == nil || threshold <= 0 || len(output) <= threshold ||
		toolName == readToolName || sessionID == "" {
		return output, nil
	}
	id := toolresult.NewID()
	preview := renderToolResultPreview(
		output,
		string(id),
		readToolName,
		min(toolResultPreviewBytes, threshold),
	)
	if len(preview) >= len(output) {
		return output, nil
	}
	if err := store.Stage(ctx, toolresult.Stage{
		ID: id, SessionID: sessionID, ToolName: toolName, Body: output,
	}); err != nil {
		return output, nil
	}
	return preview, &toolresult.Ref{ID: id}
}
