package runtime

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/agent/core"
)

// RunDeployment runs an exact deployment as a fresh root process.
// It does not require a parent process. The Deployment must belong to engine.
// Use this when infrastructure has already selected an immutable executable
// identity and must not race a later active-deployment change.
func (e *Engine) RunDeployment(
	ctx context.Context,
	deployment *Deployment,
	bindings core.Bindings,
	options core.ProcessOptions,
) (*Process, error) {
	if e == nil {
		return nil, errors.New("run deployment: engine is nil")
	}
	deployment, err := e.ownedDeployment("run deployment", deployment)
	if err != nil {
		return nil, err
	}
	return e.runDeployment(ctx, deployment, bindings, options)
}
