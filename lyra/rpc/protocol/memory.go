package protocol

import "context"

// Memory is the memory.* method group — read/write the LYRA.md
// long-term memory the engine injects into runs (API.md §5.2 / §6.12).
// Backed by the runtime's memory service (internal/service/memory).
type Memory interface {
	// ListMemory enumerates every memory entry across scopes. Empty is
	// valid (no LYRA.md yet, or no memory service configured).
	ListMemory(ctx context.Context) ([]MemoryEntry, error)

	// GetMemory returns the full LYRA.md content for one scope.
	GetMemory(ctx context.Context, scope MemoryScope) (*GetMemoryResponse, error)

	// UpdateMemory overwrites the LYRA.md for one scope.
	UpdateMemory(ctx context.Context, in UpdateMemoryRequest) error
}

// MemoryScope selects which LYRA.md a memory op targets (API.md §6.12).
type MemoryScope string

const (
	// MemoryScopeProject — `<cwd>/LYRA.md`, project-specific knowledge.
	MemoryScopeProject MemoryScope = "project"
	// MemoryScopeUser — `~/.lyra/LYRA.md`, cross-project preferences.
	MemoryScopeUser MemoryScope = "user"
)

// Valid reports whether s is a known scope — callers validate at the
// dispatch boundary and return -32602 invalid_params on false.
func (s MemoryScope) Valid() bool {
	return s == MemoryScopeProject || s == MemoryScopeUser
}

// MemoryEntry is one memory record (API.md §6.12).
type MemoryEntry struct {
	Scope      MemoryScope `json:"scope"`
	Content    string      `json:"content"`
	CapturedAt string      `json:"capturedAt,omitempty"` // RFC3339
}

// GetMemoryRequest — memory.get params.
type GetMemoryRequest struct {
	Scope MemoryScope `json:"scope"`
}

// GetMemoryResponse — memory.get result.
type GetMemoryResponse struct {
	Scope   MemoryScope `json:"scope"`
	Content string      `json:"content"`
}

// UpdateMemoryRequest — memory.update params.
type UpdateMemoryRequest struct {
	Scope   MemoryScope `json:"scope"`
	Content string      `json:"content"`
}
