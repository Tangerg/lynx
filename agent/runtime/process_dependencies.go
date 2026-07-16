package runtime

import (
	"errors"

	"github.com/Tangerg/lynx/agent/core"
)

// prepareProcessDependencies closes engine composition, validates an optional
// host-built process scope, and closes that scope before execution begins.
func (e *Engine) prepareProcessDependencies(configured *core.Dependencies) (*core.Dependencies, error) {
	e.dependencies.Freeze()
	if configured == nil {
		configured = e.dependencies.Child()
	} else if configured.Parent() != e.dependencies {
		return nil, errors.New("process dependencies must be an immediate child of engine dependencies")
	}
	configured.Freeze()
	return configured, nil
}
