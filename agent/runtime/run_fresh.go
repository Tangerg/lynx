package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// RunFresh is the top-level spawn: starts a fresh process via
// [Platform.RunAgent] (no parent process required in ctx) and binds in under
// [core.DefaultBindingName]. Used by MCP-publish style flows where each call is
// its own root process rather than a child of the calling LLM's parent.
//
// nil in produces a nil bindings map so the agent's first action resolves its
// input from the planner's defaults instead of from a `nil` slot.
func RunFresh(
	ctx context.Context,
	platform *Platform,
	agentDef *core.Agent,
	in any,
) (*AgentProcess, error) {
	if platform == nil {
		return nil, errors.New("run fresh: platform is nil")
	}
	if agentDef == nil {
		return nil, errors.New("run fresh: agent is nil")
	}
	var bindings map[string]any
	if in != nil {
		bindings = map[string]any{core.DefaultBindingName: in}
	}
	proc, err := platform.RunAgent(ctx, agentDef, bindings, core.ProcessOptions{})
	if err != nil {
		return nil, fmt.Errorf("run agent %q: %w", agentDef.Name, err)
	}
	return proc, nil
}
