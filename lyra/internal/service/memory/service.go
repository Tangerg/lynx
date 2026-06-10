// Package memory defines the MemoryService — Lyra's long-term memory
// surface. Memory is the cascade of LYRA.md files (project + user
// scopes) that get auto-injected into every session's system prompt.
package memory

import (
	"context"
	"time"
)

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
	CapturedAt time.Time // when this entry last landed in LYRA.md
}

// Service is the MemoryService contract. The implementation is
// file-backed (internal/storage) — LYRA.md stays a user-editable
// file by design, the one deliberate exception to the SQLite backend.
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
