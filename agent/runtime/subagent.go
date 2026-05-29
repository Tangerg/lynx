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
// Each tool runs its agent as a child process (inheriting the parent
// blackboard, like [AsChatTool]) and returns the child's most-recent
// blackboard object as JSON. Errors when a name isn't deployed or exposes
// no exported goal — a supervisor over an un-callable agent is a
// configuration bug worth catching at build time.
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
			out = append(out, newDynamicAgentTool(platform, agentDef, goal, SpawnChild))
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
//	tool, _ := runtime.AsChatTool[Topic, Brief](platform, "research-agent")
//	tools := []chat.Tool{tool}
//	req, _ := pc.Chat().WithTools(tools...).Call().Text(ctx)
//
// Returns an error when platform is nil, agentName is empty, or the
// agent is not registered.
func AsChatTool[In, Out any](platform *Platform, agentName string) (chat.Tool, error) {
	agentDef, err := findAgent("AsChatTool", platform, agentName)
	if err != nil {
		return nil, err
	}
	return newTypedAgentTool[In, Out]("subagent", platform, agentDef, SpawnChild), nil
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
	if err := validateAgent("AsChatToolFromAgent", platform, agentDef); err != nil {
		return nil, err
	}
	return newTypedAgentTool[In, Out]("subagent", platform, agentDef, SpawnChild), nil
}

// AsBackgroundChatTool exposes an agent as a PAIR of [chat.Tool]s
// implementing the background-task pattern (mirrors the SDK's
// AgentDefinition.background + stopTask): a spawn tool that launches the
// agent in the background and returns a task id immediately, and a
// collect tool that reports the task's status and — once it completes —
// its typed result.
//
// This is the non-blocking counterpart to [AsChatToolFromAgent]: instead
// of stalling its tool loop on every delegation, the parent LLM can
// spawn several long-running sub-tasks, keep reasoning, and collect each
// later. The child joins the parent's budget subtree and runs via
// [SpawnChildAsync]; cancel one outstanding task with
// [Platform.KillProcess], or sweep all of a turn's tasks on exit with
// [Platform.KillChildren].
//
// In is the spawn argument type (same schema as [AsChatToolFromAgent]);
// Out is the result type the collect tool extracts via
// [core.ResultOfType]. The tools are named "<agent>_spawn" /
// "<agent>_collect"; the spawn result hands the collect tool a task id.
//
// Returns an error when platform or agent is nil.
func AsBackgroundChatTool[In, Out any](platform *Platform, agentDef *core.Agent) (spawn chat.Tool, collect chat.Tool, err error) {
	if err := validateAgent("AsBackgroundChatTool", platform, agentDef); err != nil {
		return nil, nil, err
	}

	var inSample In
	spawn, err = chat.NewTool(
		chat.ToolDefinition{
			Name:        agentDef.Name + "_spawn",
			Description: "Start " + agentDef.Name + " as a background task. Returns a task_id immediately; collect its result later with " + agentDef.Name + "_collect.",
			InputSchema: pkgjson.MustStringDefSchemaOf(inSample),
		},
		chat.ToolMetadata{},
		func(ctx context.Context, arguments string) (string, error) {
			var in In
			if arguments != "" {
				if err := json.Unmarshal([]byte(arguments), &in); err != nil {
					return "", fmt.Errorf("background spawn %q: parse input: %w", agentDef.Name, err)
				}
			}
			taskID, _, err := SpawnChildAsync(ctx, platform, agentDef, in)
			if err != nil {
				return "", fmt.Errorf("background spawn %q: %w", agentDef.Name, err)
			}
			return marshalTaskResult(map[string]any{"task_id": taskID, "status": "running"})
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime.AsBackgroundChatTool: spawn tool: %w", err)
	}

	collect, err = chat.NewTool(
		chat.ToolDefinition{
			Name:        agentDef.Name + "_collect",
			Description: "Collect a background " + agentDef.Name + " task by task_id. Reports status running|waiting|done|failed, with the result when done.",
			InputSchema: pkgjson.MustStringDefSchemaOf(collectTaskInput{}),
		},
		chat.ToolMetadata{},
		func(_ context.Context, arguments string) (string, error) {
			var args collectTaskInput
			if err := json.Unmarshal([]byte(arguments), &args); err != nil {
				return "", fmt.Errorf("background collect %q: parse input: %w", agentDef.Name, err)
			}
			if args.TaskID == "" {
				return "", fmt.Errorf("background collect %q: task_id is required", agentDef.Name)
			}
			child, ok := platform.ProcessByID(args.TaskID)
			if !ok {
				return "", fmt.Errorf("background collect %q: unknown task_id %q", agentDef.Name, args.TaskID)
			}
			return collectResult[Out](agentDef.Name, child)
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime.AsBackgroundChatTool: collect tool: %w", err)
	}
	return spawn, collect, nil
}

// collectTaskInput is the argument shape of the collect tool half of
// [AsBackgroundChatTool].
type collectTaskInput struct {
	TaskID string `json:"task_id"`
}

// collectResult renders a background child's current state as a JSON
// tool-result string: "running" while the loop is live, "waiting" (with
// the pending awaitable) when parked on HITL, "done" with the typed
// result on clean completion, "failed" with the terminal reason
// otherwise. Unlike the synchronous [agentTool.Call], a failed task is a
// structured result rather than a tool error — the model explicitly
// asked for the task's outcome, so it should be able to react instead of
// aborting its loop.
func collectResult[Out any](agentName string, child *AgentProcess) (string, error) {
	switch status := child.Status(); {
	case status == core.StatusWaiting:
		return waitingResultText(agentName, child), nil
	case !status.IsTerminal():
		return marshalTaskResult(map[string]any{"task_id": child.ID(), "status": "running"})
	case status == core.StatusCompleted:
		out, ok := core.ResultOfType[Out](child)
		if !ok {
			var zero Out
			return "", fmt.Errorf("background collect %q: completed but produced no %T", agentName, zero)
		}
		return marshalTaskResult(map[string]any{"task_id": child.ID(), "status": "done", "result": out})
	default:
		reason := ""
		if err := ChildError(child); err != nil {
			reason = err.Error()
		}
		return marshalTaskResult(map[string]any{"task_id": child.ID(), "status": "failed", "error": reason})
	}
}

// marshalTaskResult JSON-encodes a task-result map for a background
// tool reply.
func marshalTaskResult(payload map[string]any) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal task result: %w", err)
	}
	return string(encoded), nil
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
	agentDef, err := findAgent("AsMCPTool", platform, agentName)
	if err != nil {
		return nil, err
	}
	return newTypedAgentTool[In, Out]("publish agent", platform, agentDef, RunFresh), nil
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

// findAgent looks the agent up by name. Used by [AsChatTool] /
// [AsMCPTool] — returns an error when the platform is nil, name is
// empty, or the agent isn't registered.
func findAgent(label string, platform *Platform, name string) (*core.Agent, error) {
	if platform == nil {
		return nil, fmt.Errorf("runtime.%s: platform must not be nil", label)
	}
	if name == "" {
		return nil, fmt.Errorf("runtime.%s: agentName must not be empty", label)
	}
	agentDef, ok := platform.FindAgent(name)
	if !ok {
		return nil, fmt.Errorf("runtime.%s: agent %q not registered on platform", label, name)
	}
	return agentDef, nil
}

// validateAgent is the [AsChatToolFromAgent] companion: same nil
// checks as [findAgent] minus the registry lookup.
func validateAgent(label string, platform *Platform, agentDef *core.Agent) error {
	if platform == nil {
		return fmt.Errorf("runtime.%s: platform must not be nil", label)
	}
	if agentDef == nil {
		return fmt.Errorf("runtime.%s: agent must not be nil", label)
	}
	return nil
}
