// Package memory defines the MemoryService — Lyra's long-term memory
// surface. Memory is the cascade of LYRA.md files (project + user
// scopes) that get auto-injected into every session's system prompt.
package memory

import "context"

// Scope selects which LYRA.md the operation targets. The runtime
// composes both at session start (project overrides user).
type Scope int

const (
	// ScopeProject — `<cwd>/LYRA.md`. Project-specific knowledge:
	// conventions, key files, gotchas.
	ScopeProject Scope = iota
	// ScopeUser — `~/.lyra/LYRA.md`. Cross-project preferences:
	// coding style, tools, vocabulary.
	ScopeUser
)

// Entry is one piece of stored memory. Content is the verbatim markdown
// shown to the model; CapturedAt records when it landed in LYRA.md.
type Entry struct {
	Scope      Scope
	Content    string
	CapturedAt string // RFC3339; kept as string so simple transports don't need to marshal time
}

// Service is the MemoryService contract. File-backed and SQLite-
// backed implementations live in internal/storage and
// internal/storage/sqlite respectively.
type Service interface {
	// Get returns the full LYRA.md content for the given scope.
	// Empty result is valid (file may not exist yet).
	Get(ctx context.Context, scope Scope) (string, error)

	// Update overwrites the LYRA.md file for the given scope with
	// the supplied markdown. Concurrent writers race; last-write-wins.
	Update(ctx context.Context, scope Scope, content string) error

	// List enumerates every memory entry across scopes. Used by UIs
	// that want to render a list rather than a flat markdown blob.
	List(ctx context.Context) ([]Entry, error)
}
