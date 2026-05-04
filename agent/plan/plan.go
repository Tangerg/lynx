package plan

import "github.com/Tangerg/lynx/agent/core"

// Plan is the planner's output: an ordered list of actions whose
// accumulated effects achieve Goal.Preconditions. An empty action list with
// a non-nil Goal means "the goal is already satisfied".
type Plan struct {
	Actions []core.Action
	Goal    *core.Goal
}

// IsComplete reports "we don't need to do any more work for this goal".
func (p *Plan) IsComplete() bool {
	return p == nil || len(p.Actions) == 0
}

// Cost is the sum of action costs; the planner uses it to rank competing
// plans. It samples each action's cost against the supplied world state so
// dynamic-cost actions get evaluated correctly. Actions with a nil Cost
// contribute nothing — the canonical construction path ([core.NewAction])
// fills in [core.Static](1.0).
func (p *Plan) Cost(ws core.WorldState) float64 {
	if p == nil {
		return 0
	}

	total := 0.0
	for _, action := range p.Actions {
		if fn := action.Metadata().Cost; fn != nil {
			total += fn(ws)
		}
	}
	return total
}

// Value is the goal value; cached here so callers don't have to
// dereference (p.Goal nil-check + Value resolution) themselves. A nil
// Goal.Value contributes 0 — [core.GoalProducing] fills in
// [core.Static](1.0).
func (p *Plan) Value(ws core.WorldState) float64 {
	if p == nil || p.Goal == nil || p.Goal.Value == nil {
		return 0
	}
	return p.Goal.Value(ws)
}

// NetValue is goal value minus plan cost — the embabel ranking heuristic.
func (p *Plan) NetValue(ws core.WorldState) float64 {
	return p.Value(ws) - p.Cost(ws)
}
