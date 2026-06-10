package runtime

import (
	"context"
	"strings"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

// blackboardDeterminer is the OBSERVE stage of the OODA loop: read
// the blackboard and return the [core.WorldState] the planner needs.
// It walks the agent's planning.System.KnownConditions(), classifies
// each condition into one of four buckets, and resolves
// accordingly:
//
//  1. type-binding key ("name:Type") — true iff the blackboard has that value
//  2. hasRun_<action>                — true iff the blackboard's condition map says so
//  3. named Condition                — call .Evaluate
//  4. plain boolean key              — read from the blackboard's condition map
type blackboardDeterminer struct {
	system     *planning.System
	blackboard core.Blackboard
	process    core.Process

	// namedConditions indexes system.Conditions by Name() so the per-tick
	// dispatch is a map lookup rather than a linear scan.
	namedConditions map[string]core.Condition
}

// newBlackboardDeterminer wires the determiner. The Process pointer is
// what gets handed to user-defined Conditions during Evaluate.
func newBlackboardDeterminer(system *planning.System, blackboard core.Blackboard, process core.Process) *blackboardDeterminer {
	namedConditions := make(map[string]core.Condition, len(system.Conditions))
	for _, condition := range system.Conditions {
		if condition == nil {
			continue
		}
		namedConditions[condition.Name()] = condition
	}
	return &blackboardDeterminer{
		system:          system,
		blackboard:      blackboard,
		process:         process,
		namedConditions: namedConditions,
	}
}

// determineWorldState produces a fresh ConditionWorldState reflecting the
// blackboard's current contents. The runtime calls this at the start of
// every tick.
func (d *blackboardDeterminer) determineWorldState(ctx context.Context) core.WorldState {
	state := map[string]core.Determination{}
	env := &core.ConditionEnv{Process: d.process, Blackboard: d.blackboard}

	for condition := range d.system.KnownConditions() {
		state[condition] = d.evaluateCondition(ctx, condition, env)
	}
	return planning.NewConditionWorldState(state)
}

// evaluateCondition dispatches to the right resolution strategy based on
// the condition key's shape. Returns Unknown for anything that doesn't
// match a known pattern — A* treats Unknown as "doesn't satisfy" so missing
// state safely defers planning rather than producing a wrong plan.
//
// User-supplied Conditions run inside [safeEvaluateCondition] so a
// panicking implementation degrades to Unknown rather than tearing down
// the whole tick — mirrors [core.ProcessContext.ExecuteSafely]'s guard
// for action bodies.
func (d *blackboardDeterminer) evaluateCondition(ctx context.Context, key string, env *core.ConditionEnv) core.Determination {
	if strings.Contains(key, ":") {
		return d.evaluateTypeBinding(key)
	}

	if strings.HasPrefix(key, core.HasRunPrefix) {
		return d.evaluateHasRun(key)
	}

	if cond, ok := d.namedConditions[key]; ok {
		return safeEvaluateCondition(ctx, cond, env)
	}

	if value, ok := d.blackboard.Condition(key); ok {
		return core.FromBool(value)
	}
	return core.Unknown
}

// safeEvaluateCondition runs cond.Evaluate under a panic guard. A
// panicking user condition becomes [core.Unknown] — A* treats Unknown
// as "doesn't satisfy", so a misbehaving condition fails its actions
// closed (planner picks something else) rather than crashing the tick.
func safeEvaluateCondition(ctx context.Context, condition core.Condition, env *core.ConditionEnv) (result core.Determination) {
	defer func() {
		if r := recover(); r != nil {
			result = core.Unknown
		}
	}()
	return condition.Evaluate(ctx, env)
}

func (d *blackboardDeterminer) evaluateTypeBinding(key string) core.Determination {
	binding := core.ParseIOBinding(key)
	return core.FromBool(d.blackboard.HasValue(binding.Name, binding.Type))
}

func (d *blackboardDeterminer) evaluateHasRun(key string) core.Determination {
	value, _ := d.blackboard.Condition(key)
	return core.FromBool(value)
}
