package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

// AsChatTool wraps a deployed agent as a [chat.CallableTool] the
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
// The child process inherits the parent's blackboard via
// [Platform.CreateChildProcess], so artifacts the parent has staged
// are visible to the child. Budget aggregation is automatic — the
// parent's [core.Process.Usage] sums the entire delegation tree.
//
// Panics on construction when platform is nil or agentName isn't
// registered: programming errors should fail at boot, not on the
// first LLM tool call.
//
// Example:
//
//	tools := []chat.Tool{
//	    runtime.AsChatTool[Topic, Brief](platform, "research-agent"),
//	    runtime.AsChatTool[Brief, BlogPost](platform, "writer-agent"),
//	}
//	req, _ := pc.Chat().WithTools(tools...).Call().Text(ctx)
func AsChatTool[In, Out any](platform *Platform, agentName string) chat.CallableTool {
	agentDef := mustFindAgent("AsChatTool", platform, agentName)
	return newTypedAgentTool[In, Out]("subagent", platform, agentDef, SpawnChild)
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
// Panics on nil platform or nil agent.
func AsChatToolFromAgent[In, Out any](platform *Platform, agentDef *core.Agent) chat.CallableTool {
	mustValidateAgent("AsChatToolFromAgent", platform, agentDef)
	return newTypedAgentTool[In, Out]("subagent", platform, agentDef, SpawnChild)
}

// AsMCPTool is the top-level companion to [AsChatTool]: it wraps a
// deployed agent as a [chat.CallableTool] that an external MCP host
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
func AsMCPTool[In, Out any](platform *Platform, agentName string) chat.CallableTool {
	agentDef := mustFindAgent("AsMCPTool", platform, agentName)
	return newTypedAgentTool[In, Out]("publish agent", platform, agentDef, RunFresh)
}

// processStarter is the shape both [SpawnChild] and [RunFresh]
// already implement: ctx + platform + agent + in → terminal
// *AgentProcess. Used by [newTypedAgentTool] / [newDynamicAgentTool]
// to swap supervisor vs top-level start strategies without
// duplicating wiring.
type processStarter func(
	ctx context.Context,
	platform *Platform,
	agentDef *core.Agent,
	in any,
) (*AgentProcess, error)

// newTypedAgentTool builds the typed flavour of [agentTool]: input
// schema derived from a zero In sample at construction; decode does a
// strict json.Unmarshal into a typed In; extract uses
// [core.ResultOfType[Out]].
func newTypedAgentTool[In, Out any](
	label string,
	platform *Platform,
	agentDef *core.Agent,
	start processStarter,
) chat.CallableTool {
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

// mustFindAgent looks the agent up by name and panics — at construction
// time, not on the first LLM tool call — when the registration is
// missing. Shared between [AsChatTool] / [AsMCPTool].
func mustFindAgent(label string, platform *Platform, name string) *core.Agent {
	if platform == nil {
		panic(fmt.Sprintf("runtime.%s: platform must not be nil", label))
	}
	if name == "" {
		panic(fmt.Sprintf("runtime.%s: agentName must not be empty", label))
	}
	agentDef, ok := platform.FindAgent(name)
	if !ok {
		panic(fmt.Sprintf("runtime.%s: agent %q not registered on platform", label, name))
	}
	return agentDef
}

// mustValidateAgent is the [AsChatToolFromAgent] companion: same nil
// checks as [mustFindAgent] minus the registry lookup.
func mustValidateAgent(label string, platform *Platform, agentDef *core.Agent) {
	if platform == nil {
		panic(fmt.Sprintf("runtime.%s: platform must not be nil", label))
	}
	if agentDef == nil {
		panic(fmt.Sprintf("runtime.%s: agent must not be nil", label))
	}
}
