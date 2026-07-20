package runtime

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/planning"
)

// worldStateReader projects blackboard contents into planner state.
// It walks the agent's planning.Domain.KnownConditions() and resolves each
// condition from the source fixed when the domain was constructed.
type worldStateReader struct {
	domain     *planning.Domain
	blackboard core.Blackboard
	process    *Process

	// namedConditions indexes domain.Conditions by Name() so the per-tick
	// dispatch is a map lookup rather than a linear scan.
	namedConditions map[string]core.Condition
}

func newWorldStateReader(domain *planning.Domain, blackboard core.Blackboard, process *Process) *worldStateReader {
	namedConditions := make(map[string]core.Condition, len(domain.Conditions()))
	for _, condition := range domain.Conditions() {
		if condition == nil {
			continue
		}
		namedConditions[condition.Name()] = condition
	}
	return &worldStateReader{
		domain:          domain,
		blackboard:      blackboard,
		process:         process,
		namedConditions: namedConditions,
	}
}

func (r *worldStateReader) read(ctx context.Context) core.WorldState {
	state := core.ConditionSet{}
	env := &core.ConditionEnv{Process: r.process, Blackboard: r.blackboard}

	for condition := range r.domain.KnownConditions() {
		state[condition.Key] = r.evaluateCondition(ctx, condition, env)
	}
	return planning.NewState(state)
}

// User-supplied Conditions run inside [safeEvaluateCondition] so a
// panicking implementation degrades to Unknown rather than tearing down
// the whole tick — mirrors the runtime action executor's panic guard
// for action bodies.
func (r *worldStateReader) evaluateCondition(ctx context.Context, ref planning.ConditionRef, env *core.ConditionEnv) core.Truth {
	switch ref.Kind {
	case planning.ConditionBinding:
		return core.TruthOf(r.blackboard.HasValue(ref.Binding.Name, ref.Binding.Type))
	case planning.ConditionEvaluator:
		condition, ok := r.namedConditions[ref.Key]
		if !ok {
			return core.Unknown
		}
		conditionEnv := *env
		conditionEnv.RunInteraction = func(ctx context.Context, input core.Interaction) (interaction.Result, error) {
			return r.process.runInteraction(ctx, core.ConditionInteractionID(ref.Key), input)
		}
		return safeEvaluateCondition(ctx, condition, &conditionEnv)
	case planning.ConditionActionRun:
		value, _ := r.blackboard.Condition(ref.Key)
		return core.TruthOf(value)
	case planning.ConditionFact:
		value, ok := r.blackboard.Condition(ref.Key)
		if !ok {
			return core.Unknown
		}
		return core.TruthOf(value)
	default:
		return core.Unknown
	}
}

// safeEvaluateCondition runs cond.Evaluate under a panic guard. A
// panicking user condition becomes [core.Unknown] — GOAP treats Unknown
// as "doesn't satisfy", so a misbehaving condition fails its actions
// closed (planner picks something else) rather than crashing the tick.
func safeEvaluateCondition(ctx context.Context, condition core.Condition, env *core.ConditionEnv) (result core.Truth) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = core.Unknown
		}
	}()
	return condition.Evaluate(ctx, env)
}
