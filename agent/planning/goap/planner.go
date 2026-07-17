package goap

import (
	"context"
	"errors"
	"slices"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

const defaultMaxExpansions = 10_000

// ErrInvalidActionCost reports a cost probe that returned a negative,
// non-finite value. Uniform-cost search requires non-negative finite edges.
var ErrInvalidActionCost = errors.New("goap: invalid action cost")

// Tracing span / attribute keys for the GOAP planner. Centralized so a
// typo at one call site is impossible and listeners have one schema to
// key off; treat as stable across releases.
const (
	spanGOAP = "agent.planner.goap"

	attrGoalName         = "agent.goal.name"
	attrActionsCount     = "agent.actions.count"
	attrGOAPAlreadySat   = "agent.goap.already_satisfied"
	attrGOAPHasProducers = "agent.goap.has_goal_producers"
	attrGOAPExpansions   = "agent.goap.expansions"
	attrGOAPFound        = "agent.goap.found"
	attrGOAPPlanLength   = "agent.goap.plan_length"
)

var plannerTracer = otel.Tracer("lynx/agent/planner")

// Planner is the concrete planner. It's stateless across PlanToGoal
// calls; safe to share across goroutines.
type Planner struct {
	maxExpansions int
}

// NewPlanner returns a planner with sensible defaults (10k node
// expansions cap). Per-call overrides go through
// [planning.Options].MaxIterations.
func NewPlanner() *Planner {
	return &Planner{maxExpansions: defaultMaxExpansions}
}

// Name is the planner's extension identifier — the value an agent's
// [core.AgentConfig.PlannerName] must match to select this planner.
func (p *Planner) Name() string { return "goap" }

// PlanToGoal runs a forward uniform-cost search over world states. Uniform
// cost is A* with h=0: less aggressive than a domain-specific heuristic, but
// correct for every non-negative action-cost model the public API permits.
func (p *Planner) PlanToGoal(
	ctx context.Context,
	start core.WorldState,
	domain *planning.Domain,
	goal *core.Goal,
	options planning.Options,
) (*planning.Plan, error) {
	if err := domain.ValidatePlanInputs(start, goal); err != nil {
		return nil, err
	}

	ctx, span := plannerTracer.Start(ctx, spanGOAP,
		trace.WithAttributes(
			attribute.String(attrGoalName, goal.Name()),
			attribute.Int(attrActionsCount, len(domain.Actions())),
		),
	)
	defer span.End()

	// fail records err on the span before returning it, so a search that hits
	// an invalid cost, context cancellation, or reconstruction error traces as
	// an error span rather than a clean one (see doc/OBSERVABILITY.md).
	fail := func(err error) (*planning.Plan, error) {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	if goal.SatisfiedBy(start) {
		span.SetAttributes(attribute.Bool(attrGOAPAlreadySat, true))
		return planning.NewPlan(nil, goal), nil
	}

	candidates := p.candidateActions(domain.Actions(), options.ExcludedActions)

	s := newSearch(start, candidates, goal, p.expansionCap(options))

	// Producer pre-check — short-circuits before search burns the expansion
	// cap chasing a goal whose required conditions no candidate action can
	// establish. It is a conservative direct-producer scan (see
	// hasGoalProducers): it does not verify that those producers are
	// themselves reachable, so it only ever rejects genuinely unreachable
	// goals and never a solvable one.
	if !s.hasGoalProducers() {
		span.SetAttributes(attribute.Bool(attrGOAPHasProducers, false))
		return nil, nil
	}

	bestGoalNode, err := s.run(ctx)
	if err != nil {
		return fail(err)
	}

	span.SetAttributes(attribute.Int(attrGOAPExpansions, s.expansions))

	if bestGoalNode == nil {
		span.SetAttributes(attribute.Bool(attrGOAPFound, false))
		return nil, nil
	}

	path, err := s.reconstructPath(bestGoalNode.state.Key())
	if err != nil {
		return fail(err)
	}

	span.SetAttributes(
		attribute.Bool(attrGOAPFound, true),
		attribute.Int(attrGOAPPlanLength, len(path)),
	)
	return planning.NewPlan(path, goal), nil
}

// expansionCap honors per-call MaxIterations when supplied, otherwise returns
// the planner default. The public option keeps its planner-neutral name.
func (p *Planner) expansionCap(options planning.Options) int {
	if options.MaxIterations > 0 {
		return options.MaxIterations
	}
	return p.maxExpansions
}

// candidateActions filters the master action list against the per-call
// exclusion set and stable-sorts so more-specific actions (those with more
// preconditions) get expanded first. Specificity-first tie ordering keeps the
// search frontier focused without affecting cost optimality.
func (p *Planner) candidateActions(actions []core.Action, excluded map[string]struct{}) []core.Action {
	out := make([]core.Action, 0, len(actions))
	for _, action := range actions {
		if action == nil {
			continue
		}
		if _, skip := excluded[action.Metadata().Name]; skip {
			continue
		}
		out = append(out, action)
	}

	slices.SortStableFunc(out, func(a, b core.Action) int {
		return len(b.Metadata().Preconditions) - len(a.Metadata().Preconditions)
	})
	return out
}
