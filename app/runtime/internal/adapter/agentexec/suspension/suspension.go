// Package suspension is the temporary old-Agent HITL adapter retained only by
// the production bridge scheduled for deletion at the P8 atomic cutover.
package suspension

import (
	"context"
	"encoding/json"

	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/interruptcodec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
)

// Interrupt requests input through the old Agent suspension protocol.
func Interrupt(ctx context.Context, key string, prompt runs.Interrupt) (interrupt.Resolution, error) {
	encoded, err := interruptcodec.EncodePrompt(prompt)
	if err != nil {
		return interrupt.Resolution{}, err
	}
	response, err := hitl.Interrupt[interruptcodec.ResolutionPayload](ctx, key, json.RawMessage(encoded))
	if err != nil {
		return interrupt.Resolution{}, err
	}
	return response.Resolution()
}

func DecodePrompt(raw []byte) (runs.Interrupt, error) {
	return interruptcodec.DecodePrompt(raw)
}

func DecodeResolution(raw []byte) (interrupt.Resolution, error) {
	return interruptcodec.DecodeResolution(raw)
}

func EncodeResolution(resolution interrupt.Resolution) (json.RawMessage, error) {
	return interruptcodec.EncodeResolution(resolution)
}
