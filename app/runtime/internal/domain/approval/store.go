package approval

import (
	"context"
	"sync"
)

// RuleStore persists approval rules. It is a dumb CRUD SPI — all matching /
// precedence lives in rule.go — so a backend only has to scope-filter on read.
// Defined here (the consumer) per DIP; the production implementation is sqlite,
// the test one is MemoryStore below.
type RuleStore interface {
	// Put upserts a rule by its id (deterministic over scope/key/tool/subject),
	// so re-remembering the same rule replaces the decision rather than piling
	// duplicates.
	Put(ctx context.Context, r Rule) error

	// Visible returns every rule reachable from a session: its session-scoped
	// rules (ScopeKey == sessionID), its project's rules (ScopeKey ==
	// projectDir), and all global rules. Any tool — the domain filters by tool.
	Visible(ctx context.Context, sessionID, projectDir string) ([]Rule, error)

	// Delete removes one rule by id; removing a missing id is not an error.
	Delete(ctx context.Context, id string) error
}

// MemoryStore is the in-process RuleStore for tests and minimal embeddings.
// Production wires the sqlite-backed store; rules here die with the process.
type MemoryStore struct {
	mu    sync.Mutex
	rules map[string]Rule // by id
}

// NewMemoryStore returns an empty in-memory rule store.
func NewMemoryStore() *MemoryStore { return &MemoryStore{rules: map[string]Rule{}} }

var _ RuleStore = (*MemoryStore)(nil)

func (m *MemoryStore) Put(_ context.Context, r Rule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rules[r.ID] = r
	return nil
}

func (m *MemoryStore) Visible(_ context.Context, sessionID, projectDir string) ([]Rule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Rule
	for _, r := range m.rules {
		if visible(r, sessionID, projectDir) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *MemoryStore) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rules, id)
	return nil
}

// visible reports whether a rule is reachable from the given session/project —
// the same scope predicate the sqlite store expresses as a WHERE clause.
func visible(r Rule, sessionID, projectDir string) bool {
	switch r.Scope {
	case ScopeSession:
		return r.ScopeKey == sessionID
	case ScopeProject:
		return projectDir != "" && r.ScopeKey == projectDir
	case ScopeGlobal:
		return true
	default:
		return false
	}
}
