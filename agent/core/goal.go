package core

import "reflect"

// Goal is a named target state. The planner's job is to find an action
// sequence whose cumulative effects satisfy Goal.Preconditions(). Multiple
// goals can coexist; the planner picks the goal whose path has the
// highest (value − cost).
type Goal struct {
	Name        string
	Description string
	Pre         []string
	Inputs      []IOBinding
	OutputType  *string

	// Value is the planner's per-tick value probe. Use [Static] for a
	// constant — e.g. `Value: core.Static(1.0)`. [GoalProducing] sets a
	// [Static](1.0) default when left nil.
	Value CostFunc

	// Tags / Examples / Export are stored as metadata for the
	// (not-yet-implemented) MCP server adapter; see EMBABEL_GAP_ANALYSIS
	// P0-1. The runtime itself doesn't read them.
	Tags     []string
	Examples []string
	Export   ExportConfig
}

// ExportConfig advertises a goal as an externally callable surface —
// used by MCP / A2A integrations to expose "agent capabilities" to
// other systems. Consumed by the MCP server adapter (EMBABEL_GAP_ANALYSIS
// P0-1) once it lands.
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

// GoalProducing builds a Goal whose precondition is "an artifact of type T
// exists on the blackboard". This is by far the most common shape — it's
// what "produce a BlogPost" looks like in DSL form. The supplied template's
// scalar fields (Description, Tags, Examples, Pre, Value, Export, …) are
// preserved; Name/Inputs/OutputType/Value default-fill when the template
// leaves them at the zero value.
//
// Build a goal with non-default fields by passing a literal:
//
//	core.GoalProducing[BlogPost](core.Goal{
//	    Description: "blog post produced",
//	    Value:       core.Static(0.8),
//	})
func GoalProducing[T any](g Goal) *Goal {
	rt := reflect.TypeOf((*T)(nil)).Elem()
	typeName := TypeFullName(rt)

	if g.Name == "" {
		g.Name = "produce_" + typeName
	}
	g.Inputs = append(g.Inputs, NewIOBinding[T](DefaultBindingName))
	g.OutputType = &typeName
	if g.Value == nil {
		g.Value = Static(1.0)
	}
	return &g
}
