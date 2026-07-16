package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

// GoalToolsFor builds supervisor-flow [tools.Tool]s for the named deployed
// agents — one tool per configured goal, with input schema derived from the
// goal tool's captured input type (so the orchestrating model sees a
// typed argument shape). Use it to assemble the tool set a supervisor
// hands its LLM; see [github.com/Tangerg/lynx/agent/workflow.Supervisor].
//
// Each tool runs its agent as a child process (fresh blackboard keeping
// only the parent's protected/ambient entries, like [NewAgentTool]) and
// returns the child's most-recent blackboard object as JSON. Errors when a
// name isn't deployed or exposes no exported goal — a supervisor over an
// un-callable agent is a configuration bug worth catching at build time.
func GoalToolsFor(engine *Engine, names ...string) ([]tools.Tool, error) {
	if engine == nil {
		return nil, errors.New("runtime.GoalToolsFor: engine is nil")
	}

	var out []tools.Tool
	for _, name := range names {
		deployment, ok := engine.catalog.activeDeployment(name)
		if !ok {
			return nil, fmt.Errorf("runtime.GoalToolsFor: agent %q not deployed", name)
		}
		agent := deployment.agent

		before := len(out)
		for _, goal := range agent.Goals() {
			if goal == nil || goal.Tool() == nil {
				continue
			}
			tool, err := newGoalTool(engine, deployment, goal, runChildDeployment)
			if err != nil {
				return nil, err
			}
			out = append(out, tool)
		}
		if len(out) == before {
			return nil, fmt.Errorf("runtime.GoalToolsFor: agent %q exposes no goal tools (set GoalConfig.Tool)", name)
		}
	}
	return out, nil
}

// NewAgentTool wraps a deployed agent as a [tools.Tool] the
// parent's LLM can invoke as just-another-tool. This is the
// "supervisor" pattern: a parent agent's body uses
// [core.ProcessContext.Prompt] to ask the LLM, the LLM
// picks one of several sub-agent tools, the tool runs the sub-agent
// synchronously inside this process, and the result feeds back into
// the LLM's tool loop.
//
// In is the type the sub-agent's first action consumes; the LLM's
// tool-call argument blob is JSON-decoded into In and bound onto the
// child's blackboard via dual-binding.
// Out is the type the sub-agent produces; NewAgentTool extracts it via
// [core.Result] and JSON-encodes it as the tool result.
//
// The child runs on a clean blackboard that keeps only the parent's
// protected ambient entries (the same policy as [Engine.RunChild]) — so the
// sub-agent starts clean and does real work, rather than short-circuiting
// on an output the parent already staged, while still seeing session
// context like the working directory. Budget aggregation is automatic —
// the parent's [core.ProcessView.Usage] sums the entire delegation tree.
//
// Example:
//
//	tool, _ := runtime.NewAgentTool[Topic, Brief](engine, "research-agent")
//	answer, err := process.Prompt(ctx, prompt, core.PromptConfig{Tools: []tools.Tool{tool}})
//
// Returns an error when engine is nil, agentName is empty, or the
// agent is not registered.
func NewAgentTool[In, Out any](engine *Engine, agentName string) (tools.Tool, error) {
	deployment, err := engine.findDeployment("NewAgentTool", agentName)
	if err != nil {
		return nil, err
	}
	return newAgentTool[In, Out]("agent tool", engine, deployment, runChildDeployment)
}

// NewStandaloneAgentTool is the top-level companion to [NewAgentTool]: it wraps a
// deployed agent as a [tools.Tool] that an external MCP host
// (Claude Desktop, Cursor, …) can drive. The returned tool spins up
// a *fresh* process per call (no parent context required) so it's
// suitable for the "agent.action(input) → output" RPC pattern.
//
// Compose with [github.com/Tangerg/lynx/mcp].Register:
//
//	server := sdkmcp.NewServer(implementation, nil)
//	lynxmcp.Register(server, runtime.NewStandaloneAgentTool[Topic, Brief](engine, "BriefingAgent"))
//
// Target per-call form is ergonomic enough on its own without a
// separate batch helper.
//
// Suspended (HITL) runs surface the same JSON "status: waiting"
// payload [NewAgentTool] uses, so an MCP host can decide to drive the
// process via [Engine.Resume] out of band.
func NewStandaloneAgentTool[In, Out any](engine *Engine, agentName string) (tools.Tool, error) {
	deployment, err := engine.findDeployment("NewStandaloneAgentTool", agentName)
	if err != nil {
		return nil, err
	}
	return newAgentTool[In, Out]("standalone agent tool", engine, deployment, runDeploymentInput)
}

// runProcessFunc runs one exact compiled deployment. Tool construction
// resolves an active route once and every call retains the captured identity.
type runProcessFunc func(
	ctx context.Context,
	engine *Engine,
	deployment *Deployment,
	input any,
) (*Process, error)

// newAgentTool builds a typed agentTool for In and Out.
func newAgentTool[In, Out any](
	label string,
	engine *Engine,
	deployment *Deployment,
	run runProcessFunc,
) (tools.Tool, error) {
	if deployment == nil || deployment.agent == nil {
		return nil, errors.New("runtime.newAgentTool: deployment is nil")
	}
	agent := deployment.agent

	var input In
	inputSchema, err := schemaFor(input)
	if err != nil {
		return nil, fmt.Errorf("runtime.newAgentTool: agent %q: %w", agent.Name(), err)
	}
	return &agentTool{
		engine:     engine,
		deployment: deployment,
		definition: chat.ToolDefinition{
			Name:        agent.Name(),
			Description: agent.Description(),
			InputSchema: json.RawMessage(inputSchema),
		},
		label: label,
		decode: func(arguments string) (any, error) {
			in, err := decodeToolArguments[In](agent.Name(), label, arguments)
			if err != nil {
				return nil, fmt.Errorf("parse input: %w", err)
			}
			return in, nil
		},
		run: func(ctx context.Context, input any) (*Process, error) {
			return run(ctx, engine, deployment, input)
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
