package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/tool"
	"github.com/Tangerg/lynx/tools"
)

// SupervisorConfig configures a [Supervisor] — an LLM-orchestration agent
// that delegates through explicitly supplied tools.
//
// Unlike the planner-driven default (where a GOAP plan sequences actions),
// a supervisor hands the chosen agents to a model as tools and lets it
// model decide which to call and in what order, ReAct-style. It is an
// opt-in pattern, not a new runtime concept: the result is a perfectly
// ordinary single-action GOAP agent whose body runs the chat tool loop, so
// it deploys, snapshots, and budgets like any other agent.
type SupervisorConfig[In, Out any] struct {
	// Name / Description identify the compiled agent.
	Name        string
	Description string

	// Tools are the exact delegates exposed to the orchestrating model. Use
	// runtime.NewAgentTool to bind deployments as child-process tools.
	Tools []tool.Tool

	// Instructions is the system prompt steering the orchestration (e.g.
	// "delegate research to research-agent, then summarize-agent").
	Instructions string

	// Render builds the user prompt from the typed input. Optional —
	// defaults to JSON-encoding the input.
	Render func(In) (string, error)

	// Parse turns the LLM's final reply into the typed Out. Required.
	Parse func(text string) (Out, error)

	// MaxToolRounds caps orchestration model calls. Zero leaves them unbounded.
	MaxToolRounds int
}

// Supervisor compiles an LLM-orchestration agent over explicit delegate tools.
// The compiled agent has one action that asks the configured chat client to
// achieve the goal using those tools, then parses the final reply
// into Out. Sub-agents run as child processes, so their cost rolls up into
// the supervisor's budget when built with runtime.NewAgentTool.
//
// At execution, the compiled agent requires a chat capability on its runtime.
// Returns an error when the static workflow configuration is invalid.
func Supervisor[In, Out any](config SupervisorConfig[In, Out]) (*core.Agent, error) {
	if config.Name == "" {
		return nil, errors.New("workflow.Supervisor: Name must not be empty")
	}
	if len(config.Tools) == 0 {
		return nil, errors.New("workflow.Supervisor: Tools must not be empty")
	}
	if config.Parse == nil {
		return nil, errors.New("workflow.Supervisor: Parse must not be nil")
	}
	if _, err := tools.NewRegistry(config.Tools...); err != nil {
		return nil, fmt.Errorf("workflow.Supervisor: Tools: %w", err)
	}
	delegates := append([]tool.Tool(nil), config.Tools...)

	render := config.Render
	if render == nil {
		render = func(input In) (string, error) {
			data, err := json.Marshal(input)
			if err != nil {
				return "", fmt.Errorf("encode input as JSON: %w", err)
			}
			return string(data), nil
		}
	}

	orchestrate := core.NewAction[In, Out](
		config.Name+"-orchestrate",
		func(ctx context.Context, process *core.ProcessContext, input In) (Out, error) {
			var zero Out
			prompt, err := render(input)
			if err != nil {
				return zero, fmt.Errorf("workflow.Supervisor %q: render input: %w", config.Name, err)
			}
			text, err := process.Prompt(ctx, prompt, core.PromptConfig{
				System:        config.Instructions,
				Tools:         delegates,
				MaxToolRounds: config.MaxToolRounds,
			})
			if err != nil {
				return zero, fmt.Errorf("workflow.Supervisor %q: %w", config.Name, err)
			}
			output, err := config.Parse(text)
			if err != nil {
				return zero, fmt.Errorf("workflow.Supervisor %q: parse response: %w", config.Name, err)
			}
			return output, nil
		},
		core.ActionConfig{
			Description: config.Description,
		},
	)

	return core.NewAgent(core.AgentConfig{
		Name:        config.Name,
		Description: config.Description,
		Actions:     []core.Action{orchestrate},
		Goals:       []*core.Goal{core.NewOutputGoal[Out](core.GoalConfig{Name: config.Name})},
	}), nil
}
