// Package knowledge defines the long-term knowledge store — Lyra's
// durable memory surface. It is the cascade of LYRA.md files (project +
// user scopes) that get auto-injected into every session's system prompt.
//
// Deliberately storage + retrieval of the editable LYRA.md files only: prompt
// composition (which scopes, in what order) lives in internal/kernel/prompt,
// not here.
package knowledge

import (
	"context"
	"time"
)

// Scope selects which LYRA.md the operation targets. The prompt
// composes both per turn — user (global) first, then project, so
// project knowledge extends and overrides the global preferences.
type Scope int

const (
	// ScopeProject — `<dir>/LYRA.md`. Project-specific knowledge:
	// conventions, key files, gotchas. Addressed by the project
	// directory passed per call (a session's cwd), so one store
	// serves every project.
	ScopeProject Scope = iota
	// ScopeUser — `~/.lyra/LYRA.md`. Cross-project preferences:
	// coding style, tools, vocabulary. The global scope; per-call
	// dir is ignored.
	ScopeUser
)

// Entry is one piece of stored memory. Content is the verbatim markdown
// shown to the model; CapturedAt records when it landed in LYRA.md.
type Entry struct {
	Scope      Scope
	Content    string
	CapturedAt time.Time // when this entry last landed in LYRA.md
}

// Store is the long-term knowledge persistence contract. The implementation is
// file-backed (internal/infra/storage) — LYRA.md stays a user-editable
// file by design, the one deliberate exception to the SQLite backend.
// dir on each method is the project directory [ScopeProject] reads
// from / writes to — a session's working directory, so one store
// serves every project. [ScopeUser] ignores it. Empty dir falls back
// to the implementation's default directory (the process cwd for the
// file-backed store), preserving single-project behavior for
// callers with no session in hand (CLI, wire requests without cwd).
type Store interface {
	// Get returns the full LYRA.md content for the given scope.
	// Empty result is valid (file may not exist yet).
	Get(ctx context.Context, scope Scope, dir string) (string, error)

	// Update overwrites the LYRA.md file for the given scope with
	// the supplied markdown. Concurrent writers race; last-write-wins.
	Update(ctx context.Context, scope Scope, dir string, content string) error

	// List enumerates every memory entry across scopes. Used by UIs
	// that want to render a list rather than a flat markdown blob.
	List(ctx context.Context, dir string) ([]Entry, error)
}
