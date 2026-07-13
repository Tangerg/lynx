// Package approvaltest provides an in-memory [approval.RuleStore] for tests.
//
// It lives in its own non-test-file package so that both the approval package's
// own black-box tests and other packages' tests (e.g. kernel/turn) share one
// fixture: nothing outside a _test.go imports it, so it never ships in the
// production binary, yet any test can import it without an import cycle.
package approvaltest

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
)

// MemoryStore is an in-process [approval.RuleStore] for tests. Rules die with
// the process; production wires the sqlite-backed store.
type MemoryStore struct {
	mu    sync.Mutex
	rules map[string]approval.Rule // by id
}

// NewMemoryStore returns an empty in-memory rule store.
func NewMemoryStore() *MemoryStore { return &MemoryStore{rules: map[string]approval.Rule{}} }

var _ approval.RuleStore = (*MemoryStore)(nil)

func (m *MemoryStore) Put(_ context.Context, r approval.Rule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rules[r.ID] = r
	return nil
}

func (m *MemoryStore) Visible(_ context.Context, sessionID, projectDir string) ([]approval.Rule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []approval.Rule
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

func (m *MemoryStore) DeleteSession(_ context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, r := range m.rules {
		if r.Scope == approval.ScopeSession && r.ScopeKey == sessionID {
			delete(m.rules, id)
		}
	}
	return nil
}

// visible reports whether a rule is reachable from the given session/project —
// the same scope predicate the sqlite store expresses as a WHERE clause.
func visible(r approval.Rule, sessionID, projectDir string) bool {
	switch r.Scope {
	case approval.ScopeSession:
		return r.ScopeKey == sessionID
	case approval.ScopeProject:
		return projectDir != "" && r.ScopeKey == projectDir
	case approval.ScopeGlobal:
		return true
	default:
		return false
	}
}
