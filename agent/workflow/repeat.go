package workflow

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
)

// repeatWorkflowConfig contains the execution mechanics shared by the
// loop-oriented builders in this package. The public builders retain their
// domain-specific configuration and only supply their state and stopping rule.
type repeatWorkflowConfig[In, Out, State any] struct {
	name              string
	description       string
	actionName        string
	actionDescription string
	doneKey           string
	goalDescription   string
	maxIterations     int

	stateBinding core.Binding
	newState     func() State
	count        func(State) int
	run          func(context.Context, *core.ProcessContext, In, State) (Out, error)
	stop         func(context.Context, In, State) bool

	snapshotState []core.Binding
}

// compileRepeatWorkflow compiles the repeated-action lifecycle once: retain
// the original input, restore or initialize snapshot iteration state, execute
// one iteration, and expose a computed terminal condition to the planner.
func compileRepeatWorkflow[In, Out, State any](config repeatWorkflowConfig[In, Out, State]) *core.Agent {
	inputState := core.NewBinding[loopInput[In]](config.name + inputStateSuffix)

	doneCondition := core.NewCondition(config.doneKey, func(ctx context.Context, env *core.ConditionEnv) core.Truth {
		state, ok := core.Last[State](env.Blackboard)
		if !ok || config.count(state) == 0 {
			return core.False
		}
		if config.count(state) >= config.maxIterations {
			return core.True
		}
		var input In
		if original, ok := core.Last[loopInput[In]](env.Blackboard); ok {
			input = original.Value
		}
		if config.stop(ctx, input, state) {
			return core.True
		}
		return core.False
	})

	action := core.NewAction[In, Out](
		config.actionName,
		func(ctx context.Context, process *core.ProcessContext, input In) (Out, error) {
			state, ok := core.Last[State](process.Blackboard())
			if !ok {
				state = config.newState()
				process.Blackboard().Store(config.stateBinding.Name, state)
				// Preserve the original input. Repeated outputs can shadow an input
				// of the same Go type on the blackboard.
				process.Blackboard().Store(inputState.Name, loopInput[In]{Value: input})
			} else if original, ok := core.Last[loopInput[In]](process.Blackboard()); ok {
				input = original.Value
			}
			return config.run(ctx, process, input, state)
		},
		core.ActionConfig{
			Description: config.actionDescription,
			Repeatable:  true,
			Effects:     []string{config.doneKey},
		},
	)

	snapshotState := make([]core.Binding, 0, 2+len(config.snapshotState))
	snapshotState = append(snapshotState, config.stateBinding, inputState)
	snapshotState = append(snapshotState, config.snapshotState...)

	return core.NewAgent(core.AgentConfig{
		Name:          config.name,
		Description:   config.description,
		Actions:       []core.Action{action},
		Conditions:    []core.Condition{doneCondition},
		SnapshotState: snapshotState,
		Goals: []*core.Goal{core.NewOutputGoal[Out](core.GoalConfig{
			Name:          config.name,
			Description:   config.goalDescription,
			Preconditions: []string{config.doneKey},
		})},
	})
}
