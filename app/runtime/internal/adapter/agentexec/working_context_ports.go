package agentexec

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
)

// KnowledgeReader is the prompt composer's read-only view of human-authored
// LYRA.md content.
type KnowledgeReader interface {
	Get(ctx context.Context, scope knowledge.Scope, dir string) (string, error)
}

// PlanReader is the prompt composer's read-only view of the current Session
// Plan.
type PlanReader interface {
	List(ctx context.Context, sessionID string) ([]plan.Step, error)
}

// AgentMemoryReader supplies the pinned memory included in every fresh root.
type AgentMemoryReader interface {
	Items(ctx context.Context, scope agentmemory.Scope, project string) ([]agentmemory.Item, error)
}

// AgentMemorySearcher supplies prompt-relevant non-pinned memory.
type AgentMemorySearcher interface {
	Search(ctx context.Context, scope agentmemory.Scope, project, query string, topK int) ([]agentmemory.Item, error)
}
