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

// NewGoal is the basic constructor — name + description; everything else
// gets filled via WithXxx.
func NewGoal(name, description string) *Goal {
	return &Goal{Name: name, Description: description, ValueStatic: 1.0}
}

// GoalProducing builds a Goal whose precondition is "an artifact of type T
// exists on the blackboard". This is by far the most common shape — it's
// what "produce a BlogPost" looks like in DSL form.
func GoalProducing[T any](description string) *Goal {
	rt := reflect.TypeOf((*T)(nil)).Elem()
	typeName := TypeFullName(rt)
	return &Goal{
		Name:        "produce_" + typeName,
		Description: description,
		Inputs:      []IoBinding{NewIoBinding[T](DefaultBinding)},
		OutputType:  &typeName,
		ValueStatic: 1.0,
	}
}

// Fluent setters — Goals are "values" in spirit; each WithXxx returns the
// pointer for chainability without copying since modifications are local
// to the construction phase.

func (g *Goal) WithName(n string) *Goal             { g.Name = n; return g }
func (g *Goal) WithDescription(d string) *Goal      { g.Description = d; return g }
func (g *Goal) WithPre(p ...string) *Goal           { g.Pre = append(g.Pre, p...); return g }
func (g *Goal) WithInputs(b ...IoBinding) *Goal     { g.Inputs = append(g.Inputs, b...); return g }
func (g *Goal) WithValue(v float64) *Goal           { g.ValueStatic = v; return g }
func (g *Goal) WithValueFn(fn CostFunc) *Goal       { g.ValueFn = fn; return g }
func (g *Goal) WithTags(t ...string) *Goal          { g.Tags = append(g.Tags, t...); return g }
func (g *Goal) WithExamples(e ...string) *Goal      { g.Examples = append(g.Examples, e...); return g }
func (g *Goal) WithExport(e ExportConfig) *Goal     { g.Export = e; return g }
