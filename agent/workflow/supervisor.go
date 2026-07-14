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
// a supervisor hands the chosen sub-agents to an LLM as tools and lets the
// model decide which to call and in what order, ReAct-style. It is an
// opt-in pattern, not a new runtime concept: the result is a perfectly
// ordinary single-action GOAP agent whose body runs the chat tool loop, so
// it deploys, snapshots, and budgets like any other agent.
type SupervisorConfig[In, Out any] struct {
	// Name / Description identify the compiled agent.
	Name        string
	Description string

	// Subagents are the deployed agent names exposed to the orchestrating
	// LLM as tools (one tool per exported goal). At least one required;
	// each must be deployed and expose an exported goal.
	Subagents []string

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
// Requires a chat client on the platform (the action errors at runtime
// otherwise). Returns an error on invalid config or an un-callable
// sub-agent (not deployed / no exported goal).
func Supervisor[In, Out any](platform *runtime.Platform, cfg SupervisorConfig[In, Out]) (*core.Agent, error) {
	if platform == nil {
		return nil, errors.New("workflow.Supervisor: platform must not be nil")
	}
	if cfg.Name == "" {
		return nil, errors.New("workflow.Supervisor: Name must not be empty")
	}
	if len(cfg.Subagents) == 0 {
		return nil, errors.New("workflow.Supervisor: at least one subagent required")
	}
	if cfg.Parse == nil {
		return nil, errors.New("workflow.Supervisor: Parse must not be nil")
	}

	tools, err := runtime.SubagentTools(platform, cfg.Subagents...)
	if err != nil {
		return nil, fmt.Errorf("workflow.Supervisor: %w", err)
	}

	render := cfg.Render
	if render == nil {
		render = func(in In) string {
			if b, err := json.Marshal(in); err == nil {
				return string(b)
			}
			return fmt.Sprintf("%v", in)
		}
	}

	orchestrate := core.NewAction[In, Out](
		cfg.Name+"-orchestrate",
		func(ctx context.Context, pc *core.ProcessContext, in In) (Out, error) {
			var zero Out
			text, err := pc.PromptRunner().
				WithTools(tools...).
				WithSystem(cfg.Instructions).
				WithMaxToolRounds(cfg.MaxToolRounds).
				Generate(ctx, render(in))
			if err != nil {
				return zero, fmt.Errorf("workflow.Supervisor %q: %w", cfg.Name, err)
			}
			return cfg.Parse(text)
		},
		core.ActionConfig{
			Description: cfg.Description,
			QoS:         singleAttempt,
		},
	)

	return core.NewAgent(core.AgentConfig{
		Name:        cfg.Name,
		Description: cfg.Description,
		Actions:     []core.Action{orchestrate},
		Goals: []*core.Goal{core.GoalProducing[Out](core.Goal{
			Name:        cfg.Name,
			Description: "produce " + core.TypeName[Out](),
		})},
	}), nil
}
