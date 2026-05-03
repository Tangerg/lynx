package runtime

import (
	"context"
	"strings"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/plan"
)

const hasRunPrefix = "hasRun_"

// worldStateDeterminer is the OBSERVE stage of the OODA loop: read the
// blackboard, return what the planner needs to know about the world.
type worldStateDeterminer interface {
	DetermineWorldState(ctx context.Context) core.WorldState
}

// blackboardDeterminer is the canonical implementation. It walks the
// agent's PlanningSystem.KnownConditions(), classifies each condition into
// one of four buckets, and resolves accordingly:
//
//  1. type-binding key ("name:Type") — true iff the blackboard has that value
//  2. hasRun_<action>                — true iff the blackboard's condition map says so
//  3. named Condition                — call .Evaluate
//  4. plain boolean key              — read from the blackboard's condition map
type blackboardDeterminer struct {
	system     *plan.PlanningSystem
	blackboard core.Blackboard
	process    core.Process
}

// newBlackboardDeterminer wires the determiner. The Process pointer is what
// gets handed to user-defined Conditions during Evaluate.
func newBlackboardDeterminer(system *plan.PlanningSystem, bb core.Blackboard, proc core.Process) *blackboardDeterminer {
	return &blackboardDeterminer{system: system, blackboard: bb, process: proc}
}

// DetermineWorldState produces a fresh ConditionWorldState reflecting the
// blackboard's current contents. The runtime calls this at the start of
// every tick.
func (d *blackboardDeterminer) DetermineWorldState(ctx context.Context) core.WorldState {
	state := map[string]core.Determination{}
	oc := &core.OperationContext{Process: d.process, Blackboard: d.blackboard}

	for cond := range d.system.KnownConditions() {
		state[cond] = d.evaluateCondition(ctx, cond, oc)
	}
	return plan.NewConditionWorldState(state)
}

// evaluateCondition dispatches to the right resolution strategy based on
// the condition key's shape. Returns Unknown for anything that doesn't
// match a known pattern — A* treats Unknown as "doesn't satisfy" so missing
// state safely defers planning rather than producing a wrong plan.
func (d *blackboardDeterminer) evaluateCondition(ctx context.Context, key string, oc *core.OperationContext) core.Determination {
	if strings.Contains(key, ":") {
		return d.evaluateTypeBinding(key)
	}

	if strings.HasPrefix(key, hasRunPrefix) {
		return d.evaluateHasRun(key)
	}

	if cond := d.findNamedCondition(key); cond != nil {
		return cond.Evaluate(ctx, oc)
	}

	if value, ok := d.blackboard.GetCondition(key); ok {
		return core.FromBool(value)
	}
	return core.Unknown
}

func (d *blackboardDeterminer) evaluateTypeBinding(key string) core.Determination {
	binding := core.ParseIoBinding(key)
	if d.blackboard.HasValue(binding.Name, binding.Type) {
		return core.True
	}
	return core.False
}

func (d *blackboardDeterminer) evaluateHasRun(key string) core.Determination {
	value, ok := d.blackboard.GetCondition(key)
	if !ok {
		return core.False
	}
	return core.FromBool(value)
}

func (d *blackboardDeterminer) findNamedCondition(name string) core.Condition {
	for _, cond := range d.system.Conditions {
		if cond.Name() == name {
			return cond
		}
	}
	return nil
}
