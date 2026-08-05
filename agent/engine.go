package agent

import (
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning/goap"
	"github.com/Tangerg/lynx/agent/planning/reactive"
	"github.com/Tangerg/lynx/agent/runtime"
)

// NewEngine constructs the framework runtime. Its zero configuration installs
// the built-in planners and otherwise uses runtime defaults.
//
// As the composition root, NewEngine registers the framework's built-in
// planners (goap, reactive) before caller extensions. Their reserved names
// cannot be replaced accidentally; duplicate registration is rejected by the
// runtime. Call [runtime.New] when constructing a runtime with a completely
// custom planner set. Other planners (htn, utility) remain opt-in.
func NewEngine(config EngineConfig) (*Engine, error) {
	extensions := make([]core.Extension, 0, 2+len(config.Extensions))
	extensions = append(extensions, goap.NewPlanner(), reactive.NewPlanner())
	config.Extensions = append(extensions, config.Extensions...)
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
