package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

// AllAchievableTools walks every deployed agent and returns a
// [chat.CallableTool] for each goal whose [core.Goal.Export] is
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
func AllAchievableTools(platform *Platform) []chat.CallableTool {
	if platform == nil {
		return nil
	}
	return collectExportedTools(platform, false /*remoteOnly*/, runAsChildAny)
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
func PublishAll(platform *Platform) []chat.CallableTool {
	if platform == nil {
		return nil
	}
	return collectExportedTools(platform, true /*remoteOnly*/, runAsTopLevelAny)
}

// runProcessFuncAny is the [runProcessFunc] shape for the dynamic
// (non-generic) wrapper. Same semantics as the typed variant — the
// only difference is `in` arrives as `any` and bb.Bind handles the
// dual-binding by reflection on the runtime type.
type runProcessFuncAny func(ctx context.Context, platform *Platform, agentDef *core.Agent, in any) (*AgentProcess, error)

// collectExportedTools is the shared core of [AllAchievableTools] and
// [PublishAll]. For each deployed agent it walks goals, filters by
// Export presence (and Export.Remote when remoteOnly), and packages
// each into a [dynamicAgentTool].
func collectExportedTools(platform *Platform, remoteOnly bool, runProc runProcessFuncAny) []chat.CallableTool {
	var out []chat.CallableTool
	for _, agentDef := range platform.Agents() {
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
			out = append(out, newDynamicAgentTool(platform, agentDef, goal, runProc))
		}
	}
	return out
}

// dynamicAgentTool is the non-generic agent-as-tool wrapper used by
// [AllAchievableTools] and [PublishAll]. It uses reflection on the
// goal's [core.GoalExport.InputSample] to derive both the JSON Schema
// (at construction) and a typed unmarshal target (at Call time).
//
// Output is whatever's most recently bound on the child blackboard
// (via [core.LastResultBindingName]), JSON-marshaled.
type dynamicAgentTool struct {
	platform   *Platform
	agent      *core.Agent
	goal       *core.Goal
	def        chat.ToolDefinition
	inputType  reflect.Type
	runProc    runProcessFuncAny
}

func newDynamicAgentTool(
	platform *Platform,
	agentDef *core.Agent,
	goal *core.Goal,
	runProc runProcessFuncAny,
) *dynamicAgentTool {
	description := goal.Description
	if goal.Export.Description != "" {
		description = goal.Export.Description
	}
	schema := pkgjson.MustStringDefSchemaOf(goal.Export.InputSample)

	var inputType reflect.Type
	if goal.Export.InputSample != nil {
		inputType = reflect.TypeOf(goal.Export.InputSample)
	}

	return &dynamicAgentTool{
		platform:  platform,
		agent:     agentDef,
		goal:      goal,
		inputType: inputType,
		def: chat.ToolDefinition{
			Name:        goal.Name,
			Description: description,
			InputSchema: schema,
		},
		runProc: runProc,
	}
}

func (t *dynamicAgentTool) Definition() chat.ToolDefinition { return t.def }
func (t *dynamicAgentTool) Metadata() chat.ToolMetadata     { return chat.ToolMetadata{} }

func (t *dynamicAgentTool) Call(ctx context.Context, arguments string) (string, error) {
	in, err := t.unmarshalTypedInput(arguments)
	if err != nil {
		return "", fmt.Errorf("publish goal %q: %w", t.goal.Name, err)
	}

	proc, err := t.runProc(ctx, t.platform, t.agent, in)
	if err != nil {
		return "", fmt.Errorf("publish goal %q: %w", t.goal.Name, err)
	}

	switch status := proc.Status(); status {
	case core.StatusCompleted:
		// fall through to result extraction
	case core.StatusWaiting:
		return waitingResultText(t.agent.Name, proc), nil
	default:
		if failure := proc.Failure(); failure != nil {
			return "", fmt.Errorf("publish goal %q (process %q) ended in %s: %w", t.goal.Name, proc.ID(), status, failure)
		}
		return "", fmt.Errorf("publish goal %q (process %q) ended in %s", t.goal.Name, proc.ID(), status)
	}

	out, ok := proc.Blackboard().GetValue(core.LastResultBindingName, "")
	if !ok {
		return "", fmt.Errorf("publish goal %q completed but blackboard has no result", t.goal.Name)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("publish goal %q: marshal output: %w", t.goal.Name, err)
	}
	return string(encoded), nil
}

// unmarshalTypedInput decodes arguments into a fresh value of
// inputType (when known) so the agent's first action receives a
// properly-typed binding rather than a generic map. Empty arguments
// yield the zero value of the type (or nil when no type is known).
func (t *dynamicAgentTool) unmarshalTypedInput(arguments string) (any, error) {
	if t.inputType == nil {
		// No InputSample → caller declared a goal export without a
		// typed shape; fall back to passing arguments through as raw
		// any so something gets bound. Rare; users SHOULD provide a
		// non-zero InputSample via [core.GoalExportFor].
		if arguments == "" {
			return nil, nil
		}
		var raw any
		if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
			return nil, fmt.Errorf("parse input: %w", err)
		}
		return raw, nil
	}

	ptr := reflect.New(t.inputType)
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), ptr.Interface()); err != nil {
			return nil, fmt.Errorf("parse input as %s: %w", t.inputType.String(), err)
		}
	}
	return ptr.Elem().Interface(), nil
}

// runAsChildAny is the dynamic-typed [runProcessFuncAny] for the
// supervisor flow ([AllAchievableTools]). Mirrors the typed
// [runAsChild] but takes `any` instead of a concrete In.
func runAsChildAny(ctx context.Context, platform *Platform, agentDef *core.Agent, in any) (*AgentProcess, error) {
	parent := core.ProcessFrom(ctx)
	if parent == nil {
		return nil, fmt.Errorf("no parent process in ctx (use core.WithProcess to inject one)")
	}
	parentProc, ok := platform.GetProcess(parent.ID())
	if !ok {
		return nil, fmt.Errorf("parent process %q not registered on platform", parent.ID())
	}

	child, err := platform.CreateChildProcess(agentDef, parentProc, core.ProcessOptions{})
	if err != nil {
		return nil, fmt.Errorf("create child: %w", err)
	}
	if in != nil {
		child.Blackboard().Bind(in)
	}

	if err := platform.ContinueProcess(ctx, child.ID()); err != nil {
		return nil, fmt.Errorf("run child %q: %w", child.ID(), err)
	}
	return child, nil
}

// runAsTopLevelAny is the dynamic-typed [runProcessFuncAny] for the
// MCP-publish flow ([PublishAll]). Mirrors the typed
// [runAsTopLevel].
func runAsTopLevelAny(ctx context.Context, platform *Platform, agentDef *core.Agent, in any) (*AgentProcess, error) {
	bindings := map[string]any{core.DefaultBindingName: in}
	if in == nil {
		bindings = nil
	}
	proc, err := platform.RunAgent(ctx, agentDef, bindings, core.ProcessOptions{})
	if err != nil {
		return nil, fmt.Errorf("run: %w", err)
	}
	return proc, nil
}
