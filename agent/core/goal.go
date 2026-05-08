package core

import "reflect"

// Goal is a named target state. The planner's job is to find an
// action sequence whose cumulative effects satisfy
// Goal.Preconditions(). Multiple goals can coexist; the planner
// picks the goal whose path has the highest (value − cost).
type Goal struct {
	Name        string
	Description string
	Pre         []string
	Inputs      []IOBinding

	// Value is the planner's per-tick value probe. [GoalProducing]
	// fills [Static](1.0) when left nil.
	Value CostFunc
}

// Preconditions merges Pre + Inputs into a single [EffectSpec]: each
// listed precondition + each typed input contributes a True
// condition the planner targets.
func (g *Goal) Preconditions() EffectSpec {
	if g == nil {
		return nil
	}
	out := EffectSpec{}
	for _, p := range g.Pre {
		out[p] = True
	}
	for _, in := range g.Inputs {
		out[in.String()] = True
	}
	return out
}

// IsSatisfiedBy reports whether ws meets every goal precondition.
// Used by planners as the "are we done?" check.
func (g *Goal) IsSatisfiedBy(ws WorldState) bool {
	if g == nil || ws == nil {
		return false
	}
	state := ws.State()
	for key, required := range g.Preconditions() {
		if state[key] != required {
			return false
		}
	}
	return true
}

// GoalProducing builds a Goal whose precondition is "an artifact of
// type T exists on the blackboard" — the canonical "produce a
// BlogPost" shape. The supplied template carries Description / Pre /
// Value; missing Name + Inputs + Value default-fill from T.
//
//	core.GoalProducing[BlogPost](core.Goal{Description: "blog post produced"})
func GoalProducing[T any](g Goal) *Goal {
	rt := reflect.TypeOf((*T)(nil)).Elem()
	typeName := TypeFullName(rt)

	if g.Name == "" {
		g.Name = "produce_" + typeName
	}
	g.Inputs = append(g.Inputs, NewIOBinding[T](DefaultBindingName))
	if g.Value == nil {
		g.Value = Static(1.0)
	}
	return &g
}
