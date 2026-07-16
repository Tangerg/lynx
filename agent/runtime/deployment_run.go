package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// RunDeployment runs an exact deployment as a fresh root process.
// It does not require a parent process and binds input under
// [core.DefaultBindingName]. The Deployment must belong to engine. Use this
// for MCP-publish and other infrastructure flows that must capture an immutable
// executable identity; the standard application path may use [Engine.Run]
// with an agent definition.
//
// A nil input produces a nil bindings map so the agent's first action resolves its
// input from the planner's defaults instead of from a `nil` slot.
func (e *Engine) RunDeployment(
	ctx context.Context,
	deployment *Deployment,
	input any,
) (*Process, error) {
	if e == nil {
		return nil, errors.New("run deployment: engine is nil")
	}
	deployment, err := e.ownedDeployment("run deployment", deployment)
	if err != nil {
		return nil, err
	}
	return runDeploymentInput(ctx, e, deployment, input)
}

func runDeploymentInput(
	ctx context.Context,
	engine *Engine,
	deployment *Deployment,
	input any,
) (*Process, error) {
	if engine == nil {
		return nil, errors.New("run deployment: engine is nil")
	}
	if deployment == nil || deployment.agent == nil {
		return nil, errors.New("run deployment: deployment is nil")
	}
	var bindings map[string]any
	if input != nil {
		bindings = map[string]any{core.DefaultBindingName: input}
	}
	process, err := engine.runDeployment(ctx, deployment, bindings, core.ProcessOptions{})
	if err != nil {
		return nil, fmt.Errorf("run agent %q: %w", deployment.agent.Name(), err)
	}
	return process, nil
}
