package sqlite

import (
	"database/sql"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

// AgentMemoryStore persists the append-only extracted fact ledger and the
// addressable memory items curated from it. The ledger is raw project-scoped
// capture; items are the curated projection reconciled from a ledger fold.
type AgentMemoryStore struct {
	db *sql.DB
}

var (
	_ agentmemory.Store = (*AgentMemoryStore)(nil)
)

// NewAgentMemoryStore binds a database opened by Open.
func NewAgentMemoryStore(db *sql.DB) *AgentMemoryStore {
	return &AgentMemoryStore{db: db}
}
