package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

// SubagentTools builds supervisor-flow [chat.Tool]s for the named deployed
// agents — one tool per exported goal (Export != nil), with input schema
// derived from the goal's InputSample (so the orchestrating LLM sees a
// typed argument shape). Use it to assemble the tool set a supervisor
// hands its LLM; see [github.com/Tangerg/lynx/agent/workflow.Supervisor].
//
// Each tool runs its agent as a child process (fresh blackboard keeping
// only the parent's protected/ambient entries, like [AsChatTool]) and
// returns the child's most-recent blackboard object as JSON. Errors when a
// name isn't deployed or exposes no exported goal — a supervisor over an
// un-callable agent is a configuration bug worth catching at build time.
func SubagentTools(platform *Platform, names ...string) ([]chat.Tool, error) {
	if platform == nil {
		return nil, errors.New("runtime.SubagentTools: platform is nil")
	}

	var out []chat.Tool
	for _, name := range names {
		agentDef, ok := platform.agents.find(name)
		if !ok {
			return nil, fmt.Errorf("runtime.SubagentTools: agent %q not deployed", name)
		}

		before := len(out)
		for _, goal := range agentDef.Goals {
			if goal == nil || goal.Export == nil {
				continue
			}
			out = append(out, newDynamicAgentTool(platform, agentDef, goal, SpawnChildProtectedOnly))
		}
		if len(out) == before {
			return nil, fmt.Errorf("runtime.SubagentTools: agent %q exposes no exported goal (set Goal.Export)", name)
		}
	}
	return out, nil
}

// AsChatTool wraps a deployed agent as a [chat.Tool] the
// parent's LLM can invoke as just-another-tool. This is the
// "supervisor" pattern: a parent agent's body uses
// [core.ProcessContext.ChatWithActionTools] to ask the LLM, the LLM
// picks one of several sub-agent tools, the tool runs the sub-agent
// synchronously inside this process, and the result feeds back into
// the LLM's tool loop.
//
// In is the type the sub-agent's first action consumes; the LLM's
// tool-call argument blob is JSON-decoded into In and bound onto the
// child's blackboard via dual-binding.
// Out is the type the sub-agent produces; AsChatTool extracts it via
// [core.ResultOfType] and JSON-encodes it as the tool result.
//
// The child runs on a FRESH blackboard that keeps only the parent's
// protected/ambient entries (via [SpawnChildProtectedOnly]) — so the
// sub-agent starts clean and does real work, rather than short-circuiting
// on an output the parent already staged, while still seeing session
// context like the working directory. Budget aggregation is automatic —
// the parent's [core.Process.Usage] sums the entire delegation tree.
//
// Panics on construction when platform is nil or agentName isn't
// registered: programming errors should fail at boot, not on the
// first LLM tool call.
//
// Example:
//
//	tool, _ := runtime.AsChatTool[Topic, Brief](platform, "research-agent")
//	tools := []chat.Tool{tool}
//	req, _ := pc.Chat().WithTools(tools...).Call().Text(ctx)
//
// Returns an error when platform is nil, agentName is empty, or the
// agent is not registered.
func AsChatTool[In, Out any](platform *Platform, agentName string) (chat.Tool, error) {
	agentDef, err := platform.findAgent("AsChatTool", agentName)
	if err != nil {
		return nil, err
	}
	return newTypedAgentTool[In, Out]("subagent", platform, agentDef, SpawnChildProtectedOnly), nil
}

// AsChatToolFromAgent is the [AsChatTool] sibling that takes a
// *core.Agent directly instead of looking up by name on the
// platform. Use when the caller already holds the agent struct (e.g.
// constructed via [agent.New(...).Build()] but not yet deployed) and
// wants to skip the registry lookup, or when the agent is in flight
// across registration races. The agent need NOT be deployed on
// platform — child processes spawned from it land on platform.procs
// the same way [AsChatTool] does.
//
// Returns an error when platform or agent is nil.
func AsChatToolFromAgent[In, Out any](platform *Platform, agentDef *core.Agent) (chat.Tool, error) {
	if err := platform.validateAgent("AsChatToolFromAgent", agentDef); err != nil {
		return nil, err
	}
	return newTypedAgentTool[In, Out]("subagent", platform, agentDef, SpawnChildProtectedOnly), nil
}

// AsMCPTool is the top-level companion to [AsChatTool]: it wraps a
// deployed agent as a [chat.Tool] that an external MCP host
// (Claude Desktop, Cursor, …) can drive. The returned tool spins up
// a *fresh* process per call (no parent context required) so it's
// suitable for the "agent.action(input) → output" RPC pattern.
//
// Compose with [github.com/Tangerg/lynx/mcp].RegisterTools:
//
//	server := sdkmcp.NewServer(impl, nil)
//	mcp.RegisterTools(server, runtime.AsMCPTool[Topic, Brief](platform, "BriefingAgent"))
//
// embabel's `PerGoalMcpExportToolCallbackPublisher` does the same
// in batch over an agent's goals; lynx's typical "one goal per
// agent" shape makes the per-call form ergonomic enough that we
// don't ship a separate batch helper.
//
// Suspended (HITL) runs surface the same JSON "status: waiting"
// payload [AsChatTool] uses, so an MCP host can decide to drive the
// process via [Platform.ResumeProcess] out of band.
func AsMCPTool[In, Out any](platform *Platform, agentName string) (chat.Tool, error) {
	agentDef, err := platform.findAgent("AsMCPTool", agentName)
	if err != nil {
		return nil, err
	}
	return newTypedAgentTool[In, Out]("publish agent", platform, agentDef, RunFresh), nil
}

// processStarter is the shape [SpawnChild], [SpawnChildProtectedOnly],
// [SpawnChildFresh] and [RunFresh] all implement: ctx + platform + agent + in → terminal
// *AgentProcess. Used by [newTypedAgentTool] / [newDynamicAgentTool]
// to swap supervisor vs top-level start strategies without
// duplicating wiring.
type processStarter func(
	ctx context.Context,
	platform *Platform,
	agentDef *core.Agent,
	in any,
) (*AgentProcess, error)

// newTypedAgentTool builds the typed flavor of [agentTool]: input
// schema derived from a zero In sample at construction; decode does a
// strict json.Unmarshal into a typed In; extract uses
// [core.ResultOfType[Out]].
func newTypedAgentTool[In, Out any](
	label string,
	platform *Platform,
	agentDef *core.Agent,
	start processStarter,
) chat.Tool {
	var inSample In
	return &agentTool{
		def: chat.ToolDefinition{
			Name:        agentDef.Name,
			Description: agentDef.Description,
			InputSchema: pkgjson.MustStringDefSchemaOf(inSample),
		},
		label: label,
		agent: agentDef,
		decode: func(arguments string) (any, error) {
			var in In
			if arguments == "" {
				return in, nil
			}
			if err := json.Unmarshal([]byte(arguments), &in); err != nil {
				return nil, fmt.Errorf("parse input: %w", err)
			}
			return in, nil
		},
		run: func(ctx context.Context, in any) (*AgentProcess, error) {
			return start(ctx, platform, agentDef, in)
		},
		extract: func(child *AgentProcess) (any, error) {
			out, ok := core.ResultOfType[Out](child)
			if !ok {
				var zero Out
				return nil, fmt.Errorf("completed but produced no %T", zero)
			}
			return out, nil
		},
	}
}
