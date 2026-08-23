package agentexec

import (
	"encoding/json"
	"fmt"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type frozenToolBinding struct {
	Definition     chat.ToolDefinition  `json:"definition"`
	SafetyClass    protocol.SafetyClass `json:"safetyClass"`
	Deferred       bool                 `json:"deferred,omitempty"`
	IntrinsicInput bool                 `json:"intrinsicInput,omitempty"`
}

func toolManifestDigest(bindings []ExecutableTool) (agent.Digest, error) {
	frozen := make([]frozenToolBinding, 0, len(bindings))
	for _, binding := range bindings {
		frozen = append(frozen, frozenToolBinding{
			Definition: binding.Tool.Definition(), SafetyClass: binding.SafetyClass,
			Deferred: binding.Deferred, IntrinsicInput: binding.IntrinsicInput,
		})
	}
	encoded, err := json.Marshal(frozen)
	if err != nil {
		return agent.Digest{}, fmt.Errorf("agentexec: encode frozen Tool manifest: %w", err)
	}
	return agent.ComputeDigest(encoded), nil
}

func partitionToolManifest(
	bindings []ExecutableTool,
) (visible []toolcontract.Tool, deferred []toolcontract.Tool) {
	visible = make([]toolcontract.Tool, 0, len(bindings))
	deferred = make([]toolcontract.Tool, 0, len(bindings))
	for _, binding := range bindings {
		if binding.Deferred {
			deferred = append(deferred, binding.Tool)
			continue
		}
		visible = append(visible, binding.Tool)
	}
	return visible, deferred
}
