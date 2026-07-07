package runtime

import (
	"context"
	"errors"
	"reflect"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

// AllAchievableTools walks every deployed agent and returns a
// [chat.Tool] for each goal whose [core.Goal.Export] is
// non-nil. Each tool is a supervisor-flow wrapper (parent process
// required in ctx — same contract as [AsChatTool]) that runs the
// agent as a child process, binds the typed input on its blackboard,
// drives the loop, and returns the most-recent blackboard object as
// JSON.
//
// Name resolution: tool name = goal.Name; description =
// Export.Description (falling back to Goal.Description).
//
// Use when a parent agent's LLM should be able to invoke any
// externally-flagged goal across all deployed agents without
// enumerating them by hand. Note that tools returned here erase the
// generic In/Out types — the agent's input is decoded from JSON via
// reflection on Export.InputSample, and the output is whatever's most
// recently bound on the child blackboard. For typed end-to-end use
// [AsChatTool] / [AsChatToolFromAgent] directly.
//
// Returned slice order is deterministic per registry-iteration order
// for a fixed platform; not stable across registrations.
func AllAchievableTools(platform *Platform) []chat.Tool {
	if platform == nil {
		return nil
	}
	return platform.collectExportedTools(false /*remoteOnly*/, SpawnChildProtectedOnly)
}

// PublishAll is [AllAchievableTools]'s top-level companion: returns
// MCP-style tools (no parent process required) for every goal whose
// [core.Goal.Export.Remote] is true. Each Call spawns a fresh
// [Platform.RunAgent] invocation.
//
// Compose with [github.com/Tangerg/lynx/mcp].RegisterTools to
// fan-publish every Export.Remote goal to an MCP server in one shot:
//
//	tools := runtime.PublishAll(platform)
//	mcp.RegisterTools(server, tools...)
//
// Output extraction is dynamic (most-recent blackboard object) — see
// [AllAchievableTools] for the type erasure caveat.
func PublishAll(platform *Platform) []chat.Tool {
	if platform == nil {
		return nil
	}
	return platform.collectExportedTools(true /*remoteOnly*/, RunFresh)
}

// collectExportedTools is the shared core of [AllAchievableTools] and
// [PublishAll]. For each deployed agent it walks goals, filters by
// Export presence (and Export.Remote when remoteOnly), and packages
// each into a [newDynamicAgentTool].
func (p *Platform) collectExportedTools(remoteOnly bool, start processStarter) []chat.Tool {
	var out []chat.Tool
	for _, agentDef := range p.Agents() {
		if agentDef == nil {
			continue
		}
		for _, goal := range agentDef.Goals {
			if goal == nil || goal.Export == nil {
				continue
			}
			if remoteOnly && !goal.Export.Remote {
				continue
			}
			out = append(out, newDynamicAgentTool(p, agentDef, goal, start))
		}
	}
	return out
}

// newDynamicAgentTool builds the dynamic flavor of [agentTool]:
// input schema derived from [core.GoalExport.InputSample] at
// construction; decode uses [reflect.New] on that sample's runtime
// type so the agent receives a properly-typed binding rather than
// `map[string]any`; extract pulls the most-recent blackboard object
// via [core.LastResultBindingName] (untyped — type-erasure trade-off
// documented on [AllAchievableTools]).
func newDynamicAgentTool(
	platform *Platform,
	agentDef *core.Agent,
	goal *core.Goal,
	start processStarter,
) chat.Tool {
	description := goal.Description
	if goal.Export.Description != "" {
		description = goal.Export.Description
	}

	var inputType reflect.Type
	if goal.Export.InputSample != nil {
		inputType = reflect.TypeOf(goal.Export.InputSample)
	}

	return &agentTool{
		platform: platform,
		def: chat.ToolDefinition{
			Name:        goal.Name,
			Description: description,
			InputSchema: pkgjson.MustStringDefSchemaOf(goal.Export.InputSample),
		},
		label: "publish goal",
		agent: agentDef,
		decode: func(arguments string) (any, error) {
			return decodeToolArgumentsForType(agentDef.Name, "publish", inputType, arguments)
		},
		run: func(ctx context.Context, in any) (*AgentProcess, error) {
			return start(ctx, platform, agentDef, in)
		},
		extract: func(child *AgentProcess) (any, error) {
			out, ok := child.Blackboard().Lookup(core.LastResultBindingName, "")
			if !ok {
				return nil, errors.New("completed but blackboard has no result")
			}
			return out, nil
		},
	}
}
