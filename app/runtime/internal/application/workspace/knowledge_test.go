package workspace

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

func TestRuntimeKnowledgeUnavailable(t *testing.T) {
	c := NewKnowledge(NewScope("", "", nil), nil)
	ctx := context.Background()

	if c.Available() {
		t.Fatal("Available = true, want false")
	}
	if _, err := c.Entries(ctx, "/repo"); !errors.Is(err, ErrKnowledgeUnavailable) {
		t.Fatalf("Entries err = %v, want ErrKnowledgeUnavailable", err)
	}
	if _, err := c.Read(ctx, knowledge.ScopeProject, "/repo"); !errors.Is(err, ErrKnowledgeUnavailable) {
		t.Fatalf("Read err = %v, want ErrKnowledgeUnavailable", err)
	}
	if err := c.Update(ctx, knowledge.ScopeUser, "", "prefs"); !errors.Is(err, ErrKnowledgeUnavailable) {
		t.Fatalf("Update err = %v, want ErrKnowledgeUnavailable", err)
	}
}

func TestRuntimeKnowledgePorts(t *testing.T) {
	ctx := context.Background()
	store := &fakeKnowledgeStore{
		entries: []knowledge.Entry{{
			Scope:   knowledge.ScopeUser,
			Content: "prefs",
		}},
		content: "project notes",
	}
	c := NewKnowledge(NewScope("", "", testPaths{}), store)

	if !c.Available() {
		t.Fatal("Available = false, want true")
	}
	entries, err := c.Entries(ctx, "/repo")
	if err != nil {
		t.Fatalf("Entries err = %v", err)
	}
	if len(entries) != 1 || entries[0].Content != "prefs" || store.listCWD != "/repo" {
		t.Fatalf("Entries = %+v, cwd = %q", entries, store.listCWD)
	}

	got, err := c.Read(ctx, knowledge.ScopeProject, "/repo")
	if err != nil {
		t.Fatalf("Read err = %v", err)
	}
	if got != "project notes" || store.getScope != knowledge.ScopeProject || store.getCWD != "/repo" {
		t.Fatalf("Read = %q, scope = %v, cwd = %q", got, store.getScope, store.getCWD)
	}

	if err := c.Update(ctx, knowledge.ScopeUser, "", "global prefs"); err != nil {
		t.Fatalf("Update err = %v", err)
	}
	if store.updateScope != knowledge.ScopeUser || store.updateCWD != "" || store.updateContent != "global prefs" {
		t.Fatalf("Update scope = %v, cwd = %q, content = %q", store.updateScope, store.updateCWD, store.updateContent)
	}
}

func TestRuntimeKnowledgeRejectsUnknownScopeBeforeDispatch(t *testing.T) {
	store := &fakeKnowledgeStore{}
	c := NewKnowledge(NewScope("", "", testPaths{}), store)
	unknown := knowledge.Scope("workspace")

	if _, err := c.Read(t.Context(), unknown, "/repo"); err == nil {
		t.Fatal("Read accepted an unknown scope")
	}
	if err := c.Update(t.Context(), unknown, "/repo", "notes"); err == nil {
		t.Fatal("Update accepted an unknown scope")
	}
	if store.getScope != "" || store.updateScope != "" {
		t.Fatalf("invalid scope reached store: get=%q update=%q", store.getScope, store.updateScope)
	}
}

type fakeKnowledgeStore struct {
	entries []knowledge.Entry
	content string

	listCWD string

	getScope knowledge.Scope
	getCWD   string

	updateScope   knowledge.Scope
	updateCWD     string
	updateContent string
}

func (s *fakeKnowledgeStore) List(_ context.Context, cwd string) ([]knowledge.Entry, error) {
	s.listCWD = cwd
	return s.entries, nil
}

func (s *fakeKnowledgeStore) Get(_ context.Context, scope knowledge.Scope, cwd string) (string, error) {
	s.getScope = scope
	s.getCWD = cwd
	return s.content, nil
}

func (s *fakeKnowledgeStore) Update(_ context.Context, scope knowledge.Scope, cwd string, content string) error {
	s.updateScope = scope
	s.updateCWD = cwd
	s.updateContent = content
	return nil
}
