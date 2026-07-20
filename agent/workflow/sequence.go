package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

const minimumSequenceAgents = 2

// Sequence compiles a deterministic chain a₁ → a₂ → ... → aₙ as a
// single deployable agent. Each sub-agent runs as a child process via
// [runtime.RunChildIsolated] in declared order; the typed output of step i
// flows onto step (i+1)'s child blackboard via [core.Blackboard.Bind],
// so adjacent sub-agents resolve each other's outputs through the
// standard dual-binding mechanism.
//
// The compiled agent has one Repeatable=false action ("{name}-pipeline")
// that drives the entire chain inside one tick, and a single goal that
// produces the final Out type. If any sub-agent terminates non-Completed,
// the action fails — the parent process transitions to StatusFailed
// with the offending child's failure as the cause.
//
// Type-chain trade-off: Go variadic generics can't express
// heterogeneous chains (only In and Out are typed; intermediate
// agent_i → agent_{i+1} type alignment is the user's responsibility,
// declared in each sub-agent's Goal/Action types). Mismatches surface
// at runtime when a step's child fails to plan from the bound input.
// Users wanting strict static type chains should compose individual
// [core.NewAction[Mid_i, Mid_{i+1}]] wrappers manually.
//
// Branch isolation: every sub-agent runs in its own child process with a fresh
// blackboard, so steps cannot see parent or peer state — only the
// immediately-prior step's typed output explicitly passed as input.
//
// Budget aggregation: child processes join the parent's budget tree
// through the engine; cumulative cost / tokens / action
// count surface on the parent's [core.ProcessView.Usage].
//
// Returns an error on missing name, fewer than 2 agents, or any nil
// agent — caller decides whether to surface, retry, or panic.
func Sequence[In, Out any](
	ctx context.Context,
	engine *runtime.Engine,
	name string,
	agents ...*core.Agent,
) (*core.Agent, error) {
	if engine == nil {
		return nil, errors.New("workflow.Sequence: engine must not be nil")
	}
	if name == "" {
		return nil, errors.New("workflow.Sequence: name must not be empty")
	}
	if len(agents) < minimumSequenceAgents {
		return nil, fmt.Errorf("workflow.Sequence: at least %d agents required, got %d", minimumSequenceAgents, len(agents))
	}
	for index, agent := range agents {
		if agent == nil {
			return nil, fmt.Errorf("workflow.Sequence: agents[%d] is nil", index)
		}
	}
	deployments := make([]*runtime.Deployment, len(agents))
	for index, agent := range agents {
		deployment, err := engine.Deploy(ctx, agent)
		if err != nil {
			return nil, fmt.Errorf("workflow.Sequence: deploy agents[%d] %q: %w", index, agent.Name(), err)
		}
		deployments[index] = deployment
	}

	pipeline := core.NewAction[In, Out](
		name+"-pipeline",
		func(ctx context.Context, _ *core.ProcessContext, input In) (Out, error) {
			var zero Out

			var current any = input
			var lastChild *runtime.Process
			for index, deployment := range deployments {
				agentName := deployment.Ref().Name
				child, err := engine.RunChildIsolated(ctx, deployment, current)
				if err != nil {
					return zero, fmt.Errorf("step %d (%s): %w", index, agentName, err)
				}
				if err := child.TerminalError(); err != nil {
					return zero, fmt.Errorf("step %d (%s): %w", index, agentName, err)
				}

				// For non-final steps, take the most-recently-bound visible
				// object as the next step's input. The standard dual-binding
				// then makes it discoverable by type on the next child's
				// blackboard.
				if index < len(deployments)-1 {
					nextInput, ok := child.Blackboard().Lookup(core.LastResultBindingName, "")
					if !ok {
						return zero, fmt.Errorf("step %d (%s) produced no output to chain forward", index, agentName)
					}
					current = nextInput
				}
				lastChild = child
			}

			output, ok := core.Result[Out](lastChild)
			if !ok {
				return zero, fmt.Errorf("final step (%s) produced no %T", lastChild.Goal().Name(), zero)
			}
			return output, nil
		},
		core.ActionConfig{
			Description: "run sub-agents in declared order",
		},
	)

	return core.NewAgent(core.AgentConfig{
		Name:    name,
		Actions: []core.Action{pipeline},
		Goals:   []*core.Goal{core.NewOutputGoal[Out](core.GoalConfig{Name: name, Description: "produce " + core.TypeName[Out]()})},
	}), nil
}
