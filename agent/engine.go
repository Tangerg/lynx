package agent

import (
	"reflect"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning/goap"
	"github.com/Tangerg/lynx/agent/planning/reactive"
	"github.com/Tangerg/lynx/agent/runtime"
)

// NewEngine constructs the framework runtime. Its zero configuration installs
// the built-in planners and otherwise uses runtime defaults.
//
// As the composition root, NewEngine registers the framework's built-in
// planners (goap, reactive) unless the caller supplied one with the same name. This keeps the
// runtime package free of any concrete planner dependency — runtime
// resolves planners purely through the [planning.Planner] interface —
// while agents requesting "goap" / "reactive" (or an empty PlannerName,
// which defaults to "goap") still work out of the box. Other planners
// (htn, utility) are opt-in via config.Extensions.
func NewEngine(config EngineConfig) (*Engine, error) {
	config.Extensions = withDefaultPlanners(config.Extensions)
	return runtime.New(config)
}

// MustNewEngine is the startup/test companion to [NewEngine].
func MustNewEngine(config EngineConfig) *Engine {
	config.Extensions = withDefaultPlanners(config.Extensions)
	return runtime.MustNew(config)
}

func withDefaultPlanners(extensions []core.Extension) []core.Extension {
	taken := make(map[string]struct{}, len(extensions))
	for _, extension := range extensions {
		if !extensionIsNil(extension) {
			taken[extension.Name()] = struct{}{}
		}
	}
	defaults := make([]core.Extension, 0, 2)
	for _, planner := range []core.Extension{goap.NewPlanner(), reactive.NewPlanner()} {
		if _, ok := taken[planner.Name()]; !ok {
			defaults = append(defaults, planner)
		}
	}
	return append(defaults, extensions...)
}

func extensionIsNil(extension core.Extension) bool {
	if extension == nil {
		return true
	}
	value := reflect.ValueOf(extension)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
