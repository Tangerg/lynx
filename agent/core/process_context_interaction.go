package core

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/agent/interaction"
)

// Interact runs a complete framework-managed model/tool interaction and
// preserves its terminal event. It fails when the ProcessContext was not
// created by a runtime with managed interaction support.
func (pc *ProcessContext) Interact(ctx context.Context, input Interaction) (interaction.Result, error) {
	if pc == nil || pc.runInteraction == nil {
		return interaction.Result{}, errors.New("agent.ProcessContext.Interact: managed interaction is not configured")
	}
	return pc.runInteraction(contextOrBackground(ctx), input)
}
