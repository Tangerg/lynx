package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

func TestRuntimeMemoryUnavailable(t *testing.T) {
	rt := &Runtime{}
	ctx := context.Background()

	if rt.HasMemory() {
		t.Fatal("HasMemory = true, want false")
	}
	if _, err := rt.ListMemoryEntries(ctx, "/repo"); !errors.Is(err, ErrMemoryUnavailable) {
		t.Fatalf("ListMemoryEntries err = %v, want ErrMemoryUnavailable", err)
	}
	if _, err := rt.Memory(ctx, knowledge.ScopeProject, "/repo"); !errors.Is(err, ErrMemoryUnavailable) {
		t.Fatalf("Memory err = %v, want ErrMemoryUnavailable", err)
	}
	if err := rt.UpdateMemory(ctx, knowledge.ScopeUser, "", "prefs"); !errors.Is(err, ErrMemoryUnavailable) {
		t.Fatalf("UpdateMemory err = %v, want ErrMemoryUnavailable", err)
	}
}

func TestRuntimeMemoryPorts(t *testing.T) {
	ctx := context.Background()
	store := &fakeMemoryStore{
		entries: []knowledge.Entry{{
			Scope:   knowledge.ScopeUser,
			Content: "prefs",
		}},
		content: "project notes",
	}
	rt := &Runtime{
		memoryList:  store,
		memoryRead:  store,
		memoryWrite: store,
	}

	if !rt.HasMemory() {
		t.Fatal("HasMemory = false, want true")
	}
	entries, err := rt.ListMemoryEntries(ctx, "/repo")
	if err != nil {
		t.Fatalf("ListMemoryEntries err = %v", err)
	}
	if len(entries) != 1 || entries[0].Content != "prefs" || store.listCwd != "/repo" {
		t.Fatalf("ListMemoryEntries = %+v, cwd = %q", entries, store.listCwd)
	}

	got, err := rt.Memory(ctx, knowledge.ScopeProject, "/repo")
	if err != nil {
		t.Fatalf("Memory err = %v", err)
	}
	if got != "project notes" || store.getScope != knowledge.ScopeProject || store.getCwd != "/repo" {
		t.Fatalf("Memory = %q, scope = %v, cwd = %q", got, store.getScope, store.getCwd)
	}

	if err := rt.UpdateMemory(ctx, knowledge.ScopeUser, "", "global prefs"); err != nil {
		t.Fatalf("UpdateMemory err = %v", err)
	}
	if store.updateScope != knowledge.ScopeUser || store.updateCwd != "" || store.updateContent != "global prefs" {
		t.Fatalf("UpdateMemory scope = %v, cwd = %q, content = %q", store.updateScope, store.updateCwd, store.updateContent)
	}
}

type fakeMemoryStore struct {
	entries []knowledge.Entry
	content string

	listCwd string

	getScope knowledge.Scope
	getCwd   string

	updateScope   knowledge.Scope
	updateCwd     string
	updateContent string
}

func (s *fakeMemoryStore) List(_ context.Context, cwd string) ([]knowledge.Entry, error) {
	s.listCwd = cwd
	return s.entries, nil
}

func (s *fakeMemoryStore) Get(_ context.Context, scope knowledge.Scope, cwd string) (string, error) {
	s.getScope = scope
	s.getCwd = cwd
	return s.content, nil
}

func (s *fakeMemoryStore) Update(_ context.Context, scope knowledge.Scope, cwd string, content string) error {
	s.updateScope = scope
	s.updateCwd = cwd
	s.updateContent = content
	return nil
}
