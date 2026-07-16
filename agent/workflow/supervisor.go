package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// SupervisorConfig configures a [Supervisor] — an LLM-orchestration agent
// that delegates to other deployed agents.
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

	// Agents names the deployments exposed to the orchestrating model as tools.
	// Each agent must expose at least one goal tool.
	Agents []string

	// Instructions is the system prompt steering the orchestration (e.g.
	// "delegate research to research-agent, then summarize-agent").
	Instructions string

	// Render builds the user prompt from the typed input. Optional —
	// defaults to JSON-encoding the input.
	Render func(In) string

	// Parse turns the LLM's final reply into the typed Out. Required.
	Parse func(text string) (Out, error)

	// MaxToolRounds caps orchestration model calls. Zero selects the target
	// tool-loop default.
	MaxToolRounds int
}

// Supervisor compiles an LLM-orchestration agent over the named sub-agents.
// The compiled agent has one action that asks the configured chat client to
// achieve the goal using the sub-agent tools, then parses the final reply
// into Out. Sub-agents run as child processes, so their cost rolls up into
// the supervisor's budget.
//
// Requires a chat client on the engine (the action errors at runtime
// otherwise). Returns an error on invalid config or an un-callable
// sub-agent (not deployed / no exported goal).
func Supervisor[In, Out any](engine *runtime.Engine, config SupervisorConfig[In, Out]) (*core.Agent, error) {
	if engine == nil {
		return nil, errors.New("workflow.Supervisor: engine must not be nil")
	}
	if config.Name == "" {
		return nil, errors.New("workflow.Supervisor: Name must not be empty")
	}
	if len(config.Agents) == 0 {
		return nil, errors.New("workflow: supervisor requires at least one agent")
	}
	if config.Parse == nil {
		return nil, errors.New("workflow.Supervisor: Parse must not be nil")
	}

	tools, err := runtime.GoalToolsFor(engine, config.Agents...)
	if err != nil {
		return nil, fmt.Errorf("workflow.Supervisor: %w", err)
	}

	render := config.Render
	if render == nil {
		render = func(input In) string {
			if data, err := json.Marshal(input); err == nil {
				return string(data)
			}
			return fmt.Sprintf("%v", input)
		}
	}

	orchestrate := core.NewAction[In, Out](
		config.Name+"-orchestrate",
		func(ctx context.Context, process *core.ProcessContext, input In) (Out, error) {
			var zero Out
			text, err := process.Prompt(ctx, render(input), core.PromptConfig{
				System:        config.Instructions,
				Tools:         tools,
				MaxToolRounds: config.MaxToolRounds,
			})
			if err != nil {
				return zero, fmt.Errorf("workflow.Supervisor %q: %w", config.Name, err)
			}
			return config.Parse(text)
		},
		core.ActionConfig{
			Description: config.Description,
		},
	)

	return core.NewAgent(core.AgentConfig{
		Name:        config.Name,
		Description: config.Description,
		Actions:     []core.Action{orchestrate},
		Goals:       []*core.Goal{core.NewOutputGoal[Out](core.GoalConfig{Name: config.Name, Description: "produce " + core.TypeName[Out]()})},
	}), nil
}
