package agentexec

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/scope/app/runtime/internal/domain/goal"
	"github.com/Tangerg/scope/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/scope/app/runtime/internal/domain/plan"
	corechat "github.com/Tangerg/scope/core/chat"
)

// KnowledgeReader is the prompt composer's read-only view of human-authored
// SCOPEAPP.md content.
type KnowledgeReader interface {
	Entries(ctx context.Context, cwd string) ([]knowledge.Entry, error)
}

// PlanReader is the prompt composer's read-only view of the current Session
// Plan.
type PlanReader interface {
	List(ctx context.Context, sessionID string) ([]plan.Step, error)
}

// GoalReader is the working-context adapter's read-only view of the current
// autonomous Goal aggregate.
type GoalReader interface {
	Current(ctx context.Context, sessionID string) (goal.Goal, bool, error)
}

// InteractionModelContextState supplies model-facing Session state whose value
// may change while an Interaction is running. It is deliberately separate from
// the frozen deployment instruction snapshot.
type InteractionModelContextState interface {
	CurrentSessionState(ctx context.Context, sessionID string) ([]corechat.Message, error)
}

// AgentMemoryReader supplies the pinned memory included in every fresh root.
type AgentMemoryReader interface {
	Items(ctx context.Context, scope agentmemory.Scope, project string) ([]agentmemory.Item, error)
}

// AgentMemorySearcher supplies prompt-relevant non-pinned memory.
type AgentMemorySearcher interface {
	Search(ctx context.Context, project, query string, topK int) ([]agentmemory.Item, error)
}
