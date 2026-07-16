package agent

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
)

// PromptJSON requests JSON matching T through the process model and tool loop.
func PromptJSON[T any](ctx context.Context, process *ProcessContext, text string, config PromptConfig) (T, error) {
	return core.PromptJSON[T](ctx, process, text, config)
}
