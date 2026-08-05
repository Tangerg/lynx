package turn

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
)

// agentEngine is the controller's complete consumer-side view of Agent execution.
// The concrete agentexec.Engine remains behind this process boundary.
type agentEngine interface {
	StartTurn(ctx context.Context, request agentexec.TurnRequest) (agentexec.TurnProcess, error)
	RestoreTurn(ctx context.Context, processID string, request agentexec.RestoreTurnRequest) (agentexec.TurnProcess, error)
	SubagentProjection(processID string) (agentexec.SubagentProjection, bool)
}
