package workflow

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// Sequence compiles a deterministic chain a₁ → a₂ → ... → aₙ as a
// single deployable agent. Each sub-agent runs as a child process via
// [runtime.SpawnChild] in declared order; the typed output of step i
// flows onto step (i+1)'s child blackboard via [core.Blackboard.Bind],
// so adjacent sub-agents resolve each other's outputs through the
// standard dual-binding mechanism.
//
// The compiled agent has one CanRerun=false action ("{name}-pipeline")
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
// Branch isolation: every sub-agent runs in its own child process with
// a Spawn'd blackboard, so peer steps cannot see each other's
// intermediate state — only the immediately-prior step's typed output.
//
// Budget aggregation: child processes join the parent's budget tree
// via [Platform.CreateChildProcess]; cumulative cost / tokens / action
// count surface on the parent's [core.Process.Usage].
//
// Returns an error on missing name, fewer than 2 agents, or any nil
// agent — caller decides whether to surface, retry, or panic.
func Sequence[In, Out any](
	platform *runtime.Platform,
	name string,
	agents ...*core.Agent,
) (*core.Agent, error) {
	if platform == nil {
		return nil, fmt.Errorf("workflow.Sequence: platform must not be nil")
	}
	if name == "" {
		return nil, fmt.Errorf("workflow.Sequence: name must not be empty")
	}
	if len(agents) < 2 {
		return nil, fmt.Errorf("workflow.Sequence: at least 2 agents required, got %d", len(agents))
	}
	for i, a := range agents {
		if a == nil {
			return nil, fmt.Errorf("workflow.Sequence: agents[%d] is nil", i)
		}
	}

	pipeline := core.NewAction[In, Out](
		name+"-pipeline",
		func(ctx context.Context, _ *core.ProcessContext, in In) (Out, error) {
			var zero Out

			var current any = in
			var lastChild *runtime.AgentProcess
			for i, sub := range agents {
				child, err := runtime.SpawnChildFresh(ctx, platform, sub, current)
				if err != nil {
					return zero, fmt.Errorf("step %d (%s): %w", i, sub.Name, err)
				}
				if err := runtime.ChildError(child); err != nil {
					return zero, fmt.Errorf("step %d (%s): %w", i, sub.Name, err)
				}

				// For non-final steps, take the most-recently-bound visible
				// object as the next step's input. The standard dual-binding
				// then makes it discoverable by type on the next child's
				// blackboard.
				if i < len(agents)-1 {
					next, ok := child.Blackboard().GetValue(core.LastResultBindingName, "")
					if !ok {
						return zero, fmt.Errorf("step %d (%s) produced no output to chain forward", i, sub.Name)
					}
					current = next
				}
				lastChild = child
			}

			out, ok := core.ResultOfType[Out](lastChild)
			if !ok {
				return zero, fmt.Errorf("final step (%s) produced no %T", lastChild.Goal().Name, zero)
			}
			return out, nil
		},
		core.ActionConfig{
			Description: "run sub-agents in declared order",
			QoS:         singleAttempt,
		},
	)

	return core.NewAgent(&core.AgentConfig{
		Name:    name,
		Actions: []core.Action{pipeline},
		Goals: []*core.Goal{core.GoalProducing[Out](core.Goal{
			Name:        name,
			Description: "produce " + core.TypeFullNameOf[Out](),
		})},
	}), nil
}
