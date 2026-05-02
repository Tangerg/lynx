package core

import "reflect"

// Goal is a named target state. The planner's job is to find an action
// sequence whose cumulative effects satisfy Goal.Preconditions(). Multiple
// goals can coexist; embabel's BestValuePlan picks the goal whose path has
// the highest (value − cost).
type Goal struct {
	Name        string
	Description string
	Pre         []string
	Inputs      []IoBinding
	OutputType  *string
	ValueStatic float64
	ValueFn     CostFunc
	Tags        []string
	Examples    []string
	Export      ExportConfig
}

// ExportConfig advertises a goal as an externally callable surface — used by
// MCP / A2A integrations to expose "agent capabilities" to other systems.
type ExportConfig struct {
	Name          string
	Remote        bool
	Local         bool
	StartingTypes []reflect.Type
}

// Preconditions merges Pre + Inputs into a single EffectSpec the planner can
// use as a target. Both contribute True conditions.
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

// Value resolves ValueFn or ValueStatic. The planner queries this to rank
// competing goals during BestValuePlan.
func (g *Goal) Value(ws WorldState) float64 {
	if g == nil {
		return 0
	}
	if g.ValueFn != nil {
		return g.ValueFn(ws)
	}
	return g.ValueStatic
}

// GoalProducing builds a Goal whose precondition is "an artifact of type T
// exists on the blackboard". This is by far the most common shape — it's
// what "produce a BlogPost" looks like in DSL form. The supplied template's
// scalar fields (Description, Tags, Examples, Pre, Value, Export, …) are
// preserved; Name/Inputs/OutputType/ValueStatic are derived from T when
// the template leaves them at the zero value.
//
// Build a goal with non-default fields by passing a literal:
//
//	core.GoalProducing[BlogPost](core.Goal{
//	    Description: "blog post produced",
//	    ValueStatic: 0.8,
//	})
func GoalProducing[T any](g Goal) *Goal {
	rt := reflect.TypeOf((*T)(nil)).Elem()
	typeName := TypeFullName(rt)

	if g.Name == "" {
		g.Name = "produce_" + typeName
	}
	g.Inputs = append(g.Inputs, NewIoBinding[T](DefaultBinding))
	g.OutputType = &typeName
	if g.ValueStatic == 0 && g.ValueFn == nil {
		g.ValueStatic = 1.0
	}
	return &g
}
