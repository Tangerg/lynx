package goap

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent2/planning"
)

const defaultMaxExpansions uint32 = 10_000

// ErrExpansionLimitReached reports that search stopped before exhausting the frontier.
// It is not equivalent to a proven unreachable Goal.
var ErrExpansionLimitReached = errors.New("goap: expansion limit reached")

// Config contains the bounded search policy for a GOAP Planner.
type Config struct {
	// MaxExpansions bounds states removed from the frontier. Zero selects a safe
	// default of 10,000 expansions.
	MaxExpansions uint32
}

// Planner performs stateless uniform-cost search and is safe for concurrent
// use after construction.
type Planner struct {
	maxExpansions uint32
}

// New constructs a GOAP Planner with an explicit or default expansion limit.
func New(config Config) *Planner {
	limit := config.MaxExpansions
	if limit == 0 {
		limit = defaultMaxExpansions
	}
	return &Planner{maxExpansions: limit}
}

// Plan finds the least-cost Action sequence that predicts satisfaction of the
// Problem Goal. An exhausted frontier returns found=false; hitting the bounded
// expansion limit returns ErrExpansionLimitReached because reachability remains
// unknown.
func (planner *Planner) Plan(ctx context.Context, problem planning.Problem) (planning.Plan, bool, error) {
	if planner == nil || planner.maxExpansions == 0 || !problem.Valid() {
		return planning.Plan{}, false, planning.ErrInvalidProblem
	}
	if err := ctx.Err(); err != nil {
		return planning.Plan{}, false, err
	}
	if problem.Goal().SatisfiedBy(problem.InitialState()) {
		plan, err := planning.NewPlan(nil, 0)
		return plan, true, err
	}
	search := newSearch(problem, planner.maxExpansions)
	if !search.hasGoalProducers() {
		return planning.Plan{}, false, nil
	}
	goal, found, err := search.run(ctx)
	if err != nil {
		return planning.Plan{}, false, err
	}
	if !found {
		return planning.Plan{}, false, nil
	}
	actions, err := search.reconstruct(goal.state.Key())
	if err != nil {
		return planning.Plan{}, false, err
	}
	plan, err := planning.NewPlan(actions, goal.cost)
	if err != nil {
		return planning.Plan{}, false, err
	}
	if err := problem.ValidatePlan(plan); err != nil {
		return planning.Plan{}, false, fmt.Errorf("goap: validate result: %w", err)
	}
	return plan, true, nil
}

var _ planning.Planner = (*Planner)(nil)
