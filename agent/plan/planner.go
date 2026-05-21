package plan

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/agent/core"
)

// PlanOptions carries per-call planner knobs. ExcludedActions is the
// runtime's "ignore this recently-replanned action so we don't loop"
// signal; MaxIterations caps internal search iteration count.
type PlanOptions struct {
	ExcludedActions map[string]struct{}
	MaxIterations   int
}

// Planner is a pure strategy: given a goal, return the action
// sequence whose effects satisfy it (or nil when unreachable).
// PlansToGoals + BestValuePlan are derived templates exposed as
// package-level functions, not interface methods, so each planner
// implementation only writes the algorithm-specific part.
//
// Planner is also a platform [core.Extension]: register one (or
// several) and the runtime resolves which one to use for a given
// process by matching the agent's [core.AgentConfig.PlannerName]
// against [core.Extension.Name].
type Planner interface {
	core.Extension

	// PlanToGoal targets one specific goal. Returns (nil, nil) when
	// no plan exists (genuinely unreachable); error only on internal
	// failure.
	PlanToGoal(
		ctx context.Context,
		start core.WorldState,
		system *PlanningSystem,
		goal *core.Goal,
		options PlanOptions,
	) (*Plan, error)
}

// CheckPlanInputs validates the trio of pointers every PlanToGoal
// implementation needs. Lift the boilerplate so each strategy
// doesn't paste its own version.
func CheckPlanInputs(start core.WorldState, system *PlanningSystem, goal *core.Goal) error {
	switch {
	case start == nil:
		return errors.New("plan: start world state is nil")
	case system == nil:
		return errors.New("plan: planning system is nil")
	case goal == nil:
		return errors.New("plan: goal is nil")
	}
	return nil
}

// PlansToGoals enumerates plans for every goal in system, sorted by
// NetValue descending. Goals returning (nil, nil) from PlanToGoal
// are dropped silently; any error short-circuits.
func PlansToGoals(
	ctx context.Context,
	p Planner,
	start core.WorldState,
	system *PlanningSystem,
	options PlanOptions,
) ([]*Plan, error) {
	if system == nil {
		return nil, errors.New("plans to goals: planning system is nil")
	}
	out := make([]*Plan, 0, len(system.Goals))
	for _, goal := range system.Goals {
		pl, err := p.PlanToGoal(ctx, start, system, goal, options)
		if err != nil {
			return nil, err
		}
		if pl == nil {
			continue
		}
		out = append(out, pl)
	}
	SortByNetValueDesc(out, start)
	return out, nil
}

// BestValuePlan is the runtime's tick-time entry: the highest-
// NetValue plan across all goals, honoring the exclusion list.
// Returns (nil, nil) when no goal is reachable.
func BestValuePlan(
	ctx context.Context,
	p Planner,
	start core.WorldState,
	system *PlanningSystem,
	options PlanOptions,
) (*Plan, error) {
	plans, err := PlansToGoals(ctx, p, start, system, options)
	if err != nil || len(plans) == 0 {
		return nil, err
	}
	return plans[0], nil
}

// Prune returns a copy of system whose Actions slice is filtered
// down to actions referenced by at least one plan reachable from
// start. Goals and Conditions are kept verbatim — the dead-code
// signal we care about is "this action can never participate in
// any plan", not "this goal is unreachable".
//
// Use cases (mirrors embabel's OptimizingGoapPlanner.prune):
//
//   - Deploy-time diagnostic — surface "agent X has N actions of
//     which K are unreachable" so the author can clean up the
//     definition or notice a misconfigured precondition.
//   - Documentation generation — strip dead actions before
//     rendering the action catalog.
//   - Repeated planning over an optimised system — the planner
//     stops considering known-dead actions tick after tick.
//
// Prune does *not* mutate system. Returns (nil, error) when the
// underlying [PlansToGoals] call fails; returns (clone-with-empty-
// actions, nil) when no goal is reachable so callers can detect
// the "every action is dead" case.
func Prune(
	ctx context.Context,
	p Planner,
	start core.WorldState,
	system *PlanningSystem,
	options PlanOptions,
) (*PlanningSystem, error) {
	if system == nil {
		return nil, errors.New("prune: planning system is nil")
	}

	plans, err := PlansToGoals(ctx, p, start, system, options)
	if err != nil {
		return nil, err
	}

	referenced := map[string]struct{}{}
	for _, plan := range plans {
		for _, action := range plan.Actions {
			if action == nil {
				continue
			}
			referenced[action.Metadata().Name] = struct{}{}
		}
	}

	kept := make([]core.Action, 0, len(referenced))
	for _, action := range system.Actions {
		if action == nil {
			continue
		}
		if _, ok := referenced[action.Metadata().Name]; ok {
			kept = append(kept, action)
		}
	}
	return NewPlanningSystem(kept, system.Goals, system.Conditions), nil
}
