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

	// Tags are short keywords surfaced to LLM-driven goal selectors
	// (autonomy.LLMRanker, etc.) so the model has a richer signal
	// than just Name + Description. Typical: ["coding", "refactor"]
	// or ["sentiment", "review"]. Optional; planner ignores them.
	Tags []string

	// Examples are sample user inputs that should match this goal —
	// few-shot anchors for LLM rankers. Optional; planner ignores
	// them. Typical: ["Refactor this Go file", "Rename the Foo type"].
	Examples []string

	// Export, when non-nil, marks this goal as externally invokable —
	// runtime helpers walk every deployed agent's goals and auto-build
	// [chat.Tool] wrappers for the ones whose Export is set.
	// Nil means "internal only; not auto-exposed". The framework's
	// reader is on the runtime side (`runtime.AllAchievableTools` /
	// `runtime.PublishAll`); leaving Export non-nil without those
	// callers wired is harmless but also means nothing happens — the
	// field exists to drive user-facing fan-out, not to gate planner
	// behaviour.
	Export *GoalExport
}

// GoalExport carries the metadata `runtime.AllAchievableTools` and
// `runtime.PublishAll` need to compile a goal into a
// [chat.Tool]. Build via [GoalExportFor] so the input type's
// schema is captured for the LLM tool definition.
type GoalExport struct {
	// Remote, when true, makes the goal eligible for top-level
	// publishing (no parent process required) — typically MCP server
	// export. When false, only the in-process supervisor variant
	// (parent's LLM tool loop) picks it up.
	Remote bool

	// Description overrides Goal.Description when surfacing the goal
	// as an externally-facing tool. Useful when the internal
	// description is too implementation-flavoured for an LLM caller.
	// Empty falls back to Goal.Description.
	Description string

	// InputSample is a zero-value of the agent's logical input type
	// (the type the agent's first action consumes / the value the
	// caller binds to start the run). Used at tool-build time to
	// derive the JSON Schema and at tool-call time to drive a typed
	// json.Unmarshal so the agent receives a properly-typed binding
	// rather than `map[string]any`.
	//
	// Build via [GoalExportFor] which captures this from a generic
	// parameter; manual construction is allowed but typo-prone.
	InputSample any
}

// GoalExportFor is the typed builder for [GoalExport]: captures a
// zero-value of In so tooling can derive the tool's JSON schema and
// drive a typed unmarshal at call time without the user passing a
// loose `any` value.
//
// Example:
//
//	core.GoalProducing[BlogPost](core.Goal{
//	    Description: "Produce a blog post about a topic",
//	    Export: core.GoalExportFor[Topic](true), // Remote=true
//	})
//
// In is the agent's logical input type (the type the first action
// consumes), NOT the goal's output type — the goal already encodes
// its output type via [GoalProducing].
func GoalExportFor[In any](remote bool) *GoalExport {
	var sample In
	return &GoalExport{
		Remote:      remote,
		InputSample: sample,
	}
}

// Preconditions merges Pre + Inputs into a single [Effects]: each
// listed precondition + each typed input contributes a True
// condition the planner targets.
func (g *Goal) Preconditions() Effects {
	if g == nil {
		return nil
	}
	out := Effects{}
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
