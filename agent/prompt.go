package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/chatclient"
)

// Prompt runs one framework-managed model interaction and decodes its final
// text with output. The caller owns the output instructions and decoder;
// Agent preserves its tool loop, lifecycle, event, and usage boundaries.
func Prompt[T any](ctx context.Context, process *ProcessContext, text string, config PromptConfig, output chatclient.Output[T]) (T, error) {
	var zero T
	if process == nil {
		return zero, errors.New("agent: prompt: process context is nil")
	}
	if err := output.Validate(); err != nil {
		return zero, fmt.Errorf("agent: prompt: validate output: %w", err)
	}
	if output.Instructions != "" {
		if text != "" {
			text += "\n\n"
		}
		text += output.Instructions
	}

	raw, err := process.Prompt(ctx, text, config)
	if err != nil {
		return zero, err
	}
	decoded, err := output.Decode(raw)
	if err != nil {
		return zero, fmt.Errorf("agent: prompt: decode output: %w", err)
	}
	return decoded, nil
}
