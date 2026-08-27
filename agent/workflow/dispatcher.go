package workflow

import (
	"context"
	"fmt"

	agent "github.com/Tangerg/scope/agent"
)

// Dispatcher is the zero-state protocol guard required by a Workflow
// Deployment. Workflow emits only Framework Effects, so Dispatch always
// reports a contract violation.
type Dispatcher struct{}

// Dispatch rejects every dispatcher-targeted Effect.
func (Dispatcher) Dispatch(
	ctx context.Context,
	request agent.EffectRequest,
	emit agent.DeltaEmitter,
) (agent.Settlement, error) {
	return agent.Settlement{}, fmt.Errorf("%w: Workflow has no dispatcher Effect protocol", ErrInvalidProtocol)
}

// ReplayPolicy denies replay because no dispatcher Effect is valid.
func (Dispatcher) ReplayPolicy(effect agent.Effect) agent.ReplayPolicy {
	return agent.ReplayPolicyNever
}

var _ agent.Dispatcher = Dispatcher{}
