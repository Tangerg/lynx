package planning

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/agent/core"
)

// Options carries per-call planner knobs. ExcludedActions is the
// runtime's "ignore this recently-replanned action to avoid looping"
// signal; MaxIterations caps internal search iteration count.
type Options struct {
	ExcludedActions map[string]struct{}
	MaxIterations   int
}

// Planner is a pure strategy: given a goal, return the action
// sequence whose effects satisfy it (or nil when unreachable).
// [Domain.Plans] and [Domain.BestPlan] are derived templates, so each planner
// implementation only writes the algorithm-specific part.
//
// Planner is also an engine [core.Extension]: register one (or
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
		state core.WorldState,
		domain *Domain,
		goal *core.Goal,
		options Options,
	) (*Plan, error)
}

// ValidatePlanInputs checks the inputs every PlanToGoal implementation needs.
func (d *Domain) ValidatePlanInputs(state core.WorldState, goal *core.Goal) error {
	switch {
	case d == nil:
		return errors.New("planning.Domain.ValidatePlanInputs: domain is nil")
	case state == nil:
		return errors.New("planning.Domain.ValidatePlanInputs: world state is nil")
	case goal == nil:
		return errors.New("planning.Domain.ValidatePlanInputs: goal is nil")
	}
	return nil
}

// Plans enumerates one plan for every reachable goal, sorted by NetValue
// descending. Goals returning (nil, nil) from PlanToGoal
// are dropped silently; any error short-circuits.
func (d *Domain) Plans(
	ctx context.Context,
	planner Planner,
	state core.WorldState,
	options Options,
) ([]*Plan, error) {
	switch {
	case d == nil:
		return nil, errors.New("planning.Domain.Plans: domain is nil")
	case planner == nil:
		return nil, errors.New("planning.Domain.Plans: planner is nil")
	case state == nil:
		return nil, errors.New("planning.Domain.Plans: world state is nil")
	}
	goals := d.Goals()
	plans := make([]*Plan, 0, len(goals))
	for _, goal := range goals {
		plan, err := planner.PlanToGoal(ctx, state, d, goal, options)
		if err != nil {
			return nil, err
		}
		if plan == nil {
			continue
		}
		plans = append(plans, plan)
	}
	sortByNetValueDesc(plans, state)
	return plans, nil
}

// BestPlan returns the highest-NetValue plan across all goals, honoring the
// exclusion list.
// Returns (nil, nil) when no goal is reachable.
func (d *Domain) BestPlan(
	ctx context.Context,
	planner Planner,
	state core.WorldState,
	options Options,
) (*Plan, error) {
	plans, err := d.Plans(ctx, planner, state, options)
	if err != nil || len(plans) == 0 {
		return nil, err
	}
	return plans[0], nil
}

// Prune returns a copy of d whose Actions slice is filtered
// down to actions referenced by at least one plan reachable from
// state. Goals and Conditions are kept verbatim — the dead-code
// signal is "this action can never participate in
// any plan", not "this goal is unreachable".
//
// Use cases (prune unreachable actions):
//
//   - Deploy-time diagnostic — surface "agent X has N actions of
//     which K are unreachable" so the author can clean up the
//     definition or notice a misconfigured precondition.
//   - Documentation generation — strip dead actions before
//     rendering the action catalog.
//   - Repeated planning over an optimized domain — the planner
//     stops considering known-dead actions tick after tick.
//
// Prune does *not* mutate d. Returns (nil, error) when the
// underlying [Domain.Plans] call fails; returns (clone-with-empty-
// actions, nil) when no goal is reachable so callers can detect
// the "every action is dead" case.
func (d *Domain) Prune(
	ctx context.Context,
	planner Planner,
	state core.WorldState,
	options Options,
) (*Domain, error) {
	if d == nil {
		return nil, errors.New("planning.Domain.Prune: domain is nil")
	}

	plans, err := d.Plans(ctx, planner, state, options)
	if err != nil {
		return nil, err
	}

	referenced := map[string]struct{}{}
	for _, plan := range plans {
		for _, action := range plan.Actions() {
			if action == nil {
				continue
			}
			referenced[action.Metadata().Name] = struct{}{}
		}
	}

	kept := make([]core.Action, 0, len(referenced))
	for _, action := range d.Actions() {
		if action == nil {
			continue
		}
		if _, ok := referenced[action.Metadata().Name]; ok {
			kept = append(kept, action)
		}
	}
	return NewDomain(kept, d.Goals(), d.Conditions()), nil
}
