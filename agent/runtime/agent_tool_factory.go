package runtime

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	agentschema "github.com/Tangerg/lynx/agent/internal/schema"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"
)

// NewAgentTool wraps one exact deployment as a typed child-process tool.
// Calls require an active parent Process in ctx, start the child with clean
// working state, and aggregate the child's usage into the parent process tree.
func NewAgentTool[In, Out any](engine *Engine, deployment *Deployment) (tool.Tool, error) {
	deployment, err := engine.ownedDeployment("NewAgentTool", deployment)
	if err != nil {
		return nil, err
	}
	return newAgentTool[In, Out](engine, deployment)
}

func newAgentTool[In, Out any](engine *Engine, deployment *Deployment) (tool.Tool, error) {
	if deployment == nil || deployment.agent == nil {
		return nil, errors.New("runtime.newAgentTool: deployment is nil")
	}
	agent := deployment.agent

	var input In
	if any(input) == nil {
		return nil, fmt.Errorf("runtime.newAgentTool: agent %q: input type must be concrete", agent.Name())
	}
	inputSchema, err := agentschema.String(input)
	if err != nil {
		return nil, fmt.Errorf("runtime.newAgentTool: agent %q: derive input schema: %w", agent.Name(), err)
	}
	return &agentTool{
		engine:     engine,
		deployment: deployment,
		definition: chat.ToolDefinition{
			Name:        agent.Name(),
			Description: agent.Description(),
			InputSchema: json.RawMessage(inputSchema),
		},
		decode: func(arguments string) (any, error) {
			var in In
			if arguments != "" {
				if err := json.Unmarshal([]byte(arguments), &in); err != nil {
					return nil, fmt.Errorf("parse input: %w", err)
				}
			}
			return in, nil
		},
		result: func(child *Process) (any, error) {
			out, ok := core.Result[Out](child)
			if !ok {
				var zero Out
				return nil, fmt.Errorf("completed but produced no %T", zero)
			}
			return out, nil
		},
	}, nil
}
