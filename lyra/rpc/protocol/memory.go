package protocol

import "context"

// Memory is the memory.* method group — LYRA.md long-term memory
// (API.md §7.7). Gated on features.memory.
type Memory interface {
	ListMemory(ctx context.Context, in WorkspaceQuery) ([]MemoryEntry, error)
	GetMemory(ctx context.Context, in GetMemoryRequest) (*MemoryEntry, error)
	UpdateMemory(ctx context.Context, in UpdateMemoryRequest) error
}

// MemoryScope selects which LYRA.md a memory op targets (API.md §4.10).
type MemoryScope string

const (
	MemoryScopeCwd         MemoryScope = "cwd"
	MemoryScopeProjectRoot MemoryScope = "projectRoot"
	MemoryScopeHome        MemoryScope = "home"
)

// Valid reports whether s is a known scope.
func (s MemoryScope) Valid() bool {
	return s == MemoryScopeCwd || s == MemoryScopeProjectRoot || s == MemoryScopeHome
}

// MemoryEntry is one memory record (API.md §4.10).
type MemoryEntry struct {
	Scope     MemoryScope `json:"scope"`
	Path      string      `json:"path"`
	Content   string      `json:"content"`
	UpdatedAt string      `json:"updatedAt,omitempty"`
}

// GetMemoryRequest — memory.get body.
type GetMemoryRequest struct {
	Scope MemoryScope `json:"scope"`
	Cwd   string      `json:"cwd,omitempty"`
}

// UpdateMemoryRequest — memory.update body.
type UpdateMemoryRequest struct {
	Scope   MemoryScope `json:"scope"`
	Cwd     string      `json:"cwd,omitempty"`
	Content string      `json:"content"`
}
