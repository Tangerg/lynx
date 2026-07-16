package htn

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

const defaultMaxRecursion = 64

// Planner is the concrete HTN planner. Library is supplied at
// construction; the planner is otherwise stateless and safe to share
// across goroutines.
type Planner struct {
	library      *Library
	maxRecursion int
}

// NewPlanner returns an HTN planner backed by library. maxRecursion
// caps the decomposition depth to guard against cyclic task graphs.
// Returns an error when library is nil.
func NewPlanner(library *Library) (*Planner, error) {
	if library == nil {
		return nil, errors.New("htn.NewPlanner: library must not be nil")
	}
	return &Planner{library: library, maxRecursion: defaultMaxRecursion}, nil
}

// Name is the planner's extension identifier — the value an agent's
// [core.AgentConfig.PlannerName] must match to select this planner.
func (p *Planner) Name() string { return "htn" }

// PlanToGoal decomposes the task whose name matches goal.Name. Goals
// without a matching task return (nil, nil) so the runtime can fall
// through to a different planner if registered.
//
// Errors are reserved for *structural* problems — exceeded recursion
// depth or a method referencing an unknown subtask name. Soft
// failures (a method's preconditions don't hold; a subtask's body
// has no applicable method) cause the planner to backtrack to the
// next sibling method, returning (nil, nil) only when every option
// is exhausted.
func (p *Planner) PlanToGoal(
	ctx context.Context,
	start core.WorldState,
	domain *planning.Domain,
	goal *core.Goal,
	options planning.Options,
) (result *planning.Plan, err error) {
	if err = planning.ValidatePlanInputs(start, domain, goal); err != nil {
		return nil, err
	}

	ctx, span := plannerTracer.Start(ctx, "htn.plan",
		trace.WithAttributes(
			attribute.String("agent.planner", "htn"),
			attribute.String("agent.goal.name", goal.Name()),
		),
	)
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else if result != nil {
			span.SetAttributes(attribute.Int("agent.plan.length", len(result.Actions())))
		}
		span.End()
	}()

	root, ok := p.library.Lookup(goal.Name())
	if !ok {
		return nil, nil
	}
	actions, _, ok, err := p.decompose(ctx, root, start, options.ExcludedActions, 0)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	result = planning.NewPlan(actions, goal)
	return result, nil
}
