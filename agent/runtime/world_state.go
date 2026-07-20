package runtime

import (
	"context"
	"strings"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/planning"
)

// worldStateReader projects blackboard contents into planner state.
// It walks the agent's planning.Domain.KnownConditions(), classifies
// each condition into one of four buckets, and resolves
// accordingly:
//
//  1. type-binding key ("name:Type") — true iff the blackboard has that value
//  2. action_ran_<action>             — true iff the blackboard's condition map says so
//  3. named Condition                — call .Evaluate
//  4. plain boolean key              — read from the blackboard's condition map
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
		state[condition] = r.evaluateCondition(ctx, condition, env)
	}
	return planning.NewState(state)
}

// evaluateCondition dispatches to the right resolution strategy based on
// the condition key's shape. Returns Unknown for anything that doesn't
// match a known pattern — GOAP treats Unknown as "doesn't satisfy" so missing
// state safely defers planning rather than producing a wrong plan.
//
// User-supplied Conditions run inside [safeEvaluateCondition] so a
// panicking implementation degrades to Unknown rather than tearing down
// the whole tick — mirrors the runtime action executor's panic guard
// for action bodies.
func (r *worldStateReader) evaluateCondition(ctx context.Context, key string, env *core.ConditionEnv) core.Truth {
	if strings.Contains(key, ":") {
		return r.evaluateTypeBinding(key)
	}

	if strings.HasPrefix(key, core.ActionRunConditionPrefix) {
		return r.evaluateHasRun(key)
	}

	if condition, ok := r.namedConditions[key]; ok {
		conditionEnv := *env
		conditionEnv.RunInteraction = func(ctx context.Context, input core.Interaction) (interaction.Result, error) {
			return r.process.runInteraction(ctx, "condition:"+key, input)
		}
		return safeEvaluateCondition(ctx, condition, &conditionEnv)
	}

	if value, ok := r.blackboard.Condition(key); ok {
		return core.TruthOf(value)
	}
	return core.Unknown
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

func (r *worldStateReader) evaluateTypeBinding(key string) core.Truth {
	binding := core.ParseBinding(key)
	return core.TruthOf(r.blackboard.HasValue(binding.Name, binding.Type))
}

func (r *worldStateReader) evaluateHasRun(key string) core.Truth {
	value, _ := r.blackboard.Condition(key)
	return core.TruthOf(value)
}
