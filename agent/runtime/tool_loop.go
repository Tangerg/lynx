package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

// ToolLoopPausedError preserves a target Runner pause checkpoint for hosts
// using the convenience PromptRunner.Generate surface.
type ToolLoopPausedError struct {
	Pause *toolloop.Pause
}

func (e *ToolLoopPausedError) Error() string {
	if e == nil || e.Pause == nil {
		return "runtime: tool loop paused"
	}
	return fmt.Sprintf("runtime: tool loop paused at %q: %s", e.Pause.ID, e.Pause.Reason)
}

func runToolLoop(
	ctx context.Context,
	model chat.Model,
	request *chat.Request,
	registry *tools.Registry,
	maxRounds int,
) (string, error) {
	runner, err := toolloop.NewRunner(model, toolloop.RunnerConfig{MaxRounds: maxRounds})
	if err != nil {
		return "", err
	}
	invocation, err := toolloop.NewInvocation(request, registry)
	if err != nil {
		return "", err
	}
	for event, runErr := range runner.Run(ctx, invocation) {
		if runErr != nil {
			return "", runErr
		}
		switch {
		case event.Kind == toolloop.EventPause:
			return "", &ToolLoopPausedError{Pause: event.Pause}
		case event.Kind == toolloop.EventModelResponse && event.Final:
			return event.Response.Text(), nil
		case event.Kind == toolloop.EventToolResult && event.Final:
			return event.ToolResult.Result, nil
		}
	}
	return "", errors.New("runtime: tool loop ended without a final event")
}
