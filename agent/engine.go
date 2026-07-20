package agent

import (
	"fmt"
	"reflect"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
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
	extensions, err := withDefaultPlanners(config.Extensions)
	if err != nil {
		return nil, fmt.Errorf("agent.NewEngine: %w", err)
	}
	config.Extensions = extensions
	return runtime.New(config)
}

// MustNewEngine is the startup/test companion to [NewEngine].
func MustNewEngine(config EngineConfig) *Engine {
	engine, err := NewEngine(config)
	if err != nil {
		panic(err)
	}
	return engine
}

func withDefaultPlanners(extensions []core.Extension) ([]core.Extension, error) {
	taken := make(map[string]struct{}, len(extensions))
	for _, extension := range extensions {
		if extensionIsNil(extension) {
			continue
		}
		name, err := facadeExtensionName(extension)
		if err != nil {
			return nil, err
		}
		taken[name] = struct{}{}
	}
	defaults := make([]core.Extension, 0, 2)
	for _, planner := range []core.Extension{goap.NewPlanner(), reactive.NewPlanner()} {
		name, err := facadeExtensionName(planner)
		if err != nil {
			return nil, err
		}
		if _, ok := taken[name]; !ok {
			defaults = append(defaults, planner)
		}
	}
	return append(defaults, extensions...), nil
}

func facadeExtensionName(extension core.Extension) (name string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("extension %T Name panicked", extension), recovered)
		}
	}()
	return extension.Name(), nil
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
