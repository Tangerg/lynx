package runtime

import (
	"context"
	"fmt"

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

func (r *worldStateReader) read(ctx context.Context) (core.WorldState, error) {
	state := core.ConditionSet{}
	env := &core.ConditionEnv{Process: r.process, Blackboard: r.blackboard}

	for condition := range r.domain.KnownConditions() {
		truth, err := r.evaluateCondition(ctx, condition, env)
		if err != nil {
			return nil, err
		}
		state[condition.Key] = truth
	}
	return planning.NewState(state), nil
}

// User-supplied Conditions run inside [safeEvaluateCondition] so a
// panicking implementation degrades to Unknown rather than tearing down
// the whole tick — mirrors the runtime action executor's panic guard
// for action bodies.
func (r *worldStateReader) evaluateCondition(ctx context.Context, ref planning.ConditionRef, env *core.ConditionEnv) (core.Truth, error) {
	switch ref.Kind {
	case planning.ConditionBinding:
		return core.TruthOf(r.blackboard.HasValue(ref.Binding.Name, ref.Binding.Type)), nil
	case planning.ConditionEvaluator:
		condition, ok := r.namedConditions[ref.Key]
		if !ok {
			return core.Unknown, fmt.Errorf("runtime: condition %q has no evaluator", ref.Key)
		}
		conditionEnv := *env
		conditionEnv.RunInteraction = func(ctx context.Context, input core.Interaction) (interaction.Result, error) {
			return r.process.runInteraction(ctx, core.ConditionInteractionID(ref.Key), input)
		}
		truth := safeEvaluateCondition(ctx, condition, &conditionEnv)
		if !truth.Valid() {
			return core.Unknown, fmt.Errorf("runtime: condition %q returned invalid truth value %d", ref.Key, truth)
		}
		return truth, nil
	case planning.ConditionActionRun:
		value, _ := r.blackboard.Condition(ref.Key)
		return core.TruthOf(value), nil
	case planning.ConditionFact:
		value, ok := r.blackboard.Condition(ref.Key)
		if !ok {
			return core.Unknown, nil
		}
		return core.TruthOf(value), nil
	default:
		return core.Unknown, fmt.Errorf("runtime: condition %q has invalid source kind %d", ref.Key, ref.Kind)
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
