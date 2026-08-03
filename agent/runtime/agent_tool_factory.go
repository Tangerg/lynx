package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
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
	t := &agentTool{
		engine:     engine,
		deployment: deployment,
		result: func(child *Process) (any, error) {
			out, ok := core.Result[Out](child)
			if !ok {
				var zero Out
				return nil, fmt.Errorf("completed but produced no %T", zero)
			}
			return out, nil
		},
	}
	inner, err := tool.NewFunc[In, string](
		tool.FuncConfig{Name: agent.Name(), Description: agent.Description()},
		func(ctx context.Context, in In) (string, error) {
			return t.call(ctx, any(in))
		},
	)
	if err != nil {
		return nil, fmt.Errorf("runtime.newAgentTool: agent %q: build typed tool: %w", agent.Name(), err)
	}
	t.inner = inner
	return t, nil
}
