package planning

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/Tangerg/lynx/agent/core"
)

var errInvalidPlan = errors.New("planning: invalid plan")

const (
	// GOAPPlannerName selects the built-in goal-oriented planner.
	GOAPPlannerName = "goap"
	// HTNPlannerName selects the hierarchical task-network planner.
	HTNPlannerName = "htn"
	// ReactivePlannerName selects the one-step reactive planner.
	ReactivePlannerName = "reactive"
	// UtilityPlannerName selects the classic utility planner.
	UtilityPlannerName = "utility"
	// GoalFirstUtilityPlannerName selects the goal-first utility variant.
	GoalFirstUtilityPlannerName = "goal-first-utility"

	// DefaultPlannerName is used when AgentConfig.PlannerName is empty.
	DefaultPlannerName = GOAPPlannerName

	// TracerName and the attribute keys form the shared planner telemetry schema.
	TracerName     = "lynx/agent/planner"
	PlannerNameKey = "agent.planner"
	GoalNameKey    = "agent.goal.name"
	PlanLengthKey  = "agent.plan.length"
)

// EffectivePlannerName returns name, or [DefaultPlannerName] when name is
// empty. Runtime selection and deployment identity both use this function so
// cache identity cannot drift from execution behavior.
func EffectivePlannerName(name string) string {
	if name == "" {
		return DefaultPlannerName
	}
	return name
}

// Options carries per-call planner knobs. ExcludedActions is the
// runtime's "ignore this recently-replanned action to avoid looping"
// signal; MaxIterations caps internal search iteration count.
type Options struct {
	ExcludedActions Exclusions
	MaxIterations   int
}

// Exclusions is an immutable set of action names a planner must ignore.
// Its zero value excludes nothing.
type Exclusions struct {
	names map[string]struct{}
}

// NewExclusions returns an exclusion set containing names.
func NewExclusions(names ...string) Exclusions {
	if len(names) == 0 {
		return Exclusions{}
	}
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return Exclusions{names: set}
}

// Contains reports whether name is excluded.
func (e Exclusions) Contains(name string) bool {
	_, ok := e.names[name]
	return ok
}

// With returns an independent set that also excludes name.
func (e Exclusions) With(name string) Exclusions {
	names := maps.Clone(e.names)
	if names == nil {
		names = make(map[string]struct{})
	}
	names[name] = struct{}{}
	return Exclusions{names: names}
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
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		plan, err := planner.PlanToGoal(ctx, state, d, goal, options)
		if err != nil {
			return nil, fmt.Errorf("planning.Domain.Plans: planner %q goal %q: %w", planner.Name(), goal.Name(), err)
		}
		if plan == nil {
			continue
		}
		accepted, err := d.acceptPlan(plan, goal, state, options)
		if err != nil {
			return nil, fmt.Errorf("planning.Domain.Plans: planner %q goal %q: %w", planner.Name(), goal.Name(), err)
		}
		plans = append(plans, accepted)
	}
	sortByNetValueDesc(plans, state)
	return plans, nil
}

func (d *Domain) acceptPlan(plan *Plan, goal *core.Goal, state core.WorldState, options Options) (*Plan, error) {
	if plan.Goal() != goal {
		return nil, fmt.Errorf("%w: result targets a different goal", errInvalidPlan)
	}

	actions := plan.Actions()
	canonical := make([]core.Action, len(actions))
	cursor := state
	for index, candidate := range actions {
		if candidate == nil {
			return nil, fmt.Errorf("%w: action[%d] is nil", errInvalidPlan, index)
		}
		name := candidate.Metadata().Name
		action, ok := d.action(name)
		if !ok {
			return nil, fmt.Errorf("%w: action[%d] %q is outside the domain", errInvalidPlan, index, name)
		}
		if options.ExcludedActions.Contains(name) {
			return nil, fmt.Errorf("%w: action[%d] %q is excluded", errInvalidPlan, index, name)
		}
		metadata := action.Metadata()
		if !metadata.Applicable(cursor.Conditions()) {
			return nil, fmt.Errorf("%w: action[%d] %q has unsatisfied preconditions", errInvalidPlan, index, name)
		}
		canonical[index] = action
		cursor = cursor.Apply(metadata.Effects)
	}
	if !goal.SatisfiedBy(cursor) {
		return nil, fmt.Errorf("%w: declared effects do not satisfy goal %q", errInvalidPlan, goal.Name())
	}
	return NewPlan(canonical, goal), nil
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
	return NewDomain(kept, d.Goals(), d.Conditions())
}
