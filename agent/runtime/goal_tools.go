package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
	"github.com/Tangerg/lynx/tools"
)

func schemaFor(sample any) (string, error) {
	if sample == nil {
		return "", errors.New("input sample must not be nil")
	}
	schema, err := pkgjson.StringDefSchemaOf(sample)
	if err != nil {
		return "", fmt.Errorf("derive input schema: %w", err)
	}
	return schema, nil
}

// GoalTools walks every deployed agent and returns a
// [tools.Tool] for each goal whose [core.Goal.Tool] is
// non-nil. Each tool is a supervisor-flow wrapper (parent process
// required in ctx — same contract as [NewAgentTool]) that runs the
// agent as a child process, binds the typed input on its blackboard,
// drives the loop, and returns the most-recent blackboard object as
// JSON.
//
// Name resolution: tool name = goal.Name; description =
// GoalTool.Description (falling back to Goal.Description).
//
// Use when a parent agent's LLM should be able to invoke any
// externally-flagged goal across all deployed agents without
// enumerating them by hand. Note that tools returned here erase the
// generic In/Out types — the agent's input is decoded from JSON via
// reflection on GoalTool.InputType, and the output is whatever's most
// recently bound on the child blackboard. For typed end-to-end use
// [NewAgentTool] directly.
//
// Returned slice order is deterministic by deployed agent name, then goal
// declaration order.
func (e *Engine) GoalTools() ([]tools.Tool, error) {
	if e == nil {
		return nil, errors.New("runtime.Engine.GoalTools: engine is nil")
	}
	return e.collectGoalTools(false, runChildDeployment)
}

// StandaloneGoalTools is [Engine.GoalTools]'s top-level companion: returns
// MCP-style tools (no parent process required) for every goal whose
// [core.GoalTool.Standalone] is true. Each Call starts a fresh
// [Engine.Run] invocation.
//
// Compose with [github.com/Tangerg/lynx/mcp].Register to
// register every standalone goal with an MCP server in one shot:
//
//	tools, err := engine.StandaloneGoalTools()
//	if err != nil { ... }
//	lynxmcp.Register(server, tools...)
//
// Output extraction is dynamic (most-recent blackboard object) — see
// [Engine.GoalTools] for the type erasure caveat.
func (e *Engine) StandaloneGoalTools() ([]tools.Tool, error) {
	if e == nil {
		return nil, errors.New("runtime.Engine.StandaloneGoalTools: engine is nil")
	}
	return e.collectGoalTools(true, runRootToolDeployment)
}

func (e *Engine) collectGoalTools(standaloneOnly bool, run runProcessFunc) ([]tools.Tool, error) {
	var out []tools.Tool
	for _, deployment := range e.catalog.listActive() {
		if deployment == nil || deployment.agent == nil {
			continue
		}
		agent := deployment.agent
		for _, goal := range agent.Goals() {
			if goal == nil || goal.Tool() == nil {
				continue
			}
			if standaloneOnly && !goal.Tool().Standalone {
				continue
			}
			tool, err := newGoalTool(e, deployment, goal, run)
			if err != nil {
				return nil, err
			}
			out = append(out, tool)
		}
	}
	return out, nil
}

// newGoalTool builds a reflection-backed agentTool from goal metadata.
func newGoalTool(
	engine *Engine,
	deployment *Deployment,
	goal *core.Goal,
	run runProcessFunc,
) (tools.Tool, error) {
	if deployment == nil || deployment.agent == nil {
		return nil, errors.New("runtime.newGoalTool: deployment is nil")
	}
	agent := deployment.agent

	goalTool, err := compileGoalTool(agent, goal)
	if err != nil {
		return nil, err
	}

	return &agentTool{
		engine:     engine,
		deployment: deployment,
		definition: goalTool.definition,
		label:      "goal tool",
		decode:     goalTool.decode,
		run: func(ctx context.Context, input any) (*Process, error) {
			return run(ctx, engine, deployment, input)
		},
		result: goalTool.result,
	}, nil
}

type compiledGoalTool struct {
	agentName  string
	definition chat.ToolDefinition
	inputType  reflect.Type
}

func compileGoalTool(agent *core.Agent, goal *core.Goal) (compiledGoalTool, error) {
	config := goal.Tool()
	if config == nil {
		return compiledGoalTool{}, fmt.Errorf("runtime.compileGoalTool: goal %q has no tool configuration", goal.Name())
	}

	description := goal.Description()
	if config.Description != "" {
		description = config.Description
	}

	inputType := config.InputType()
	if inputType == nil || inputType.Kind() == reflect.Interface {
		return compiledGoalTool{}, fmt.Errorf("runtime.compileGoalTool: goal %q input type must not be an interface", goal.Name())
	}
	inputSchema, err := schemaFor(reflect.Zero(inputType).Interface())
	if err != nil {
		return compiledGoalTool{}, fmt.Errorf("runtime.compileGoalTool: goal %q: %w", goal.Name(), err)
	}

	return compiledGoalTool{
		agentName: agent.Name(),
		definition: chat.ToolDefinition{
			Name:        goal.Name(),
			Description: description,
			InputSchema: json.RawMessage(inputSchema),
		},
		inputType: inputType,
	}, nil
}

func (g compiledGoalTool) decode(arguments string) (any, error) {
	return decodeDynamicToolArguments(g.agentName, "goal tool", g.inputType, arguments)
}

func (g compiledGoalTool) result(child *Process) (any, error) {
	out, ok := child.Blackboard().Lookup(core.LastResultBindingName, "")
	if !ok {
		return nil, errors.New("completed but blackboard has no result")
	}
	return out, nil
}

// GoalToolsFor builds supervisor-flow tools for the named deployed agents. It
// returns an error when an agent is missing or exposes no configured goal tool.
func (e *Engine) GoalToolsFor(names ...string) ([]tools.Tool, error) {
	if e == nil {
		return nil, errors.New("runtime.Engine.GoalToolsFor: engine is nil")
	}

	var out []tools.Tool
	for _, name := range names {
		deployment, ok := e.catalog.activeDeployment(name)
		if !ok {
			return nil, fmt.Errorf("runtime.Engine.GoalToolsFor: agent %q not deployed", name)
		}
		agent := deployment.agent

		before := len(out)
		for _, goal := range agent.Goals() {
			if goal == nil || goal.Tool() == nil {
				continue
			}
			tool, err := newGoalTool(e, deployment, goal, runChildDeployment)
			if err != nil {
				return nil, err
			}
			out = append(out, tool)
		}
		if len(out) == before {
			return nil, fmt.Errorf("runtime.Engine.GoalToolsFor: agent %q exposes no goal tools (set GoalConfig.Tool)", name)
		}
	}
	return out, nil
}

// NewAgentTool wraps a deployed agent as a typed tool for supervisor flows.
// The child inherits protected ambient state, runs synchronously, and
// contributes usage to its parent's budget.
func NewAgentTool[In, Out any](engine *Engine, agentName string) (tools.Tool, error) {
	deployment, err := engine.findDeployment("NewAgentTool", agentName)
	if err != nil {
		return nil, err
	}
	return newAgentTool[In, Out]("agent tool", engine, deployment, runChildDeployment)
}

// NewStandaloneAgentTool wraps a deployed agent as a top-level typed tool. A
// call starts a fresh process and therefore requires no parent process context.
func NewStandaloneAgentTool[In, Out any](engine *Engine, agentName string) (tools.Tool, error) {
	deployment, err := engine.findDeployment("NewStandaloneAgentTool", agentName)
	if err != nil {
		return nil, err
	}
	return newAgentTool[In, Out]("standalone agent tool", engine, deployment, runRootToolDeployment)
}

func runRootToolDeployment(
	ctx context.Context,
	engine *Engine,
	deployment *Deployment,
	input any,
) (*Process, error) {
	var bindings core.Bindings
	if input != nil {
		bindings = core.Input(input)
	}
	return engine.RunDeployment(ctx, deployment, bindings, core.ProcessOptions{})
}

// runProcessFunc runs one exact compiled deployment. Tool construction
// resolves an active route once and every call retains the captured identity.
type runProcessFunc func(
	ctx context.Context,
	engine *Engine,
	deployment *Deployment,
	input any,
) (*Process, error)

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
