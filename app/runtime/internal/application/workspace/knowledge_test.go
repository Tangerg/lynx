package workspace

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

func TestRuntimeKnowledgeUnavailable(t *testing.T) {
	c := NewKnowledge(NewScope("", "", nil), nil, nil, nil, nil)
	ctx := context.Background()

	if c.Available() {
		t.Fatal("Available = true, want false")
	}
	if _, err := c.Entries(ctx, "/repo"); !errors.Is(err, ErrKnowledgeUnavailable) {
		t.Fatalf("Entries err = %v, want ErrKnowledgeUnavailable", err)
	}
	if _, err := c.Read(ctx, knowledge.ScopeCWD, "/repo"); !errors.Is(err, ErrKnowledgeUnavailable) {
		t.Fatalf("Read err = %v, want ErrKnowledgeUnavailable", err)
	}
	if _, err := c.Update(ctx, knowledge.ScopeHome, "", "rev-1", "prefs"); !errors.Is(err, ErrKnowledgeUnavailable) {
		t.Fatalf("Update err = %v, want ErrKnowledgeUnavailable", err)
	}
}

func TestRuntimeKnowledgePorts(t *testing.T) {
	ctx := context.Background()
	var notices []invalidation.Notice
	store := &fakeKnowledgeStore{
		entries: []knowledge.Entry{{
			Scope:   knowledge.ScopeHome,
			Content: "prefs",
		}},
		entry: knowledge.Entry{Scope: knowledge.ScopeProjectRoot, Content: "project notes", Revision: "rev-1"},
	}
	c := NewKnowledge(
		NewScope("", "", testPaths{}),
		knowledgeInspector{resolved: Resolved{Path: "/repo/work", ProjectRoot: "/repo"}},
		store, nil, func(notice invalidation.Notice) { notices = append(notices, notice) },
	)

	if !c.Available() {
		t.Fatal("Available = false, want true")
	}
	entries, err := c.Entries(ctx, "/repo/work")
	if err != nil {
		t.Fatalf("Entries err = %v", err)
	}
	if len(entries) != 1 || entries[0].Content != "prefs" || store.listCWD != "/repo/work" || store.listProjectRoot != "/repo" {
		t.Fatalf("Entries = %+v, cwd = %q, projectRoot = %q", entries, store.listCWD, store.listProjectRoot)
	}

	got, err := c.Read(ctx, knowledge.ScopeProjectRoot, "/repo/work")
	if err != nil {
		t.Fatalf("Read err = %v", err)
	}
	if got.Content != "project notes" || got.Revision != "rev-1" || store.getScope != knowledge.ScopeProjectRoot || store.getCWD != "/repo" {
		t.Fatalf("Read = %+v, scope = %v, cwd = %q", got, store.getScope, store.getCWD)
	}

	written, err := c.Update(ctx, knowledge.ScopeHome, "", "rev-1", "global prefs")
	if err != nil {
		t.Fatalf("Update err = %v", err)
	}
	if written.Content != "global prefs" || store.updateScope != knowledge.ScopeHome || store.updateCWD != "" || store.updateRevision != "rev-1" || store.updateContent != "global prefs" {
		t.Fatalf("Update scope = %v, cwd = %q, content = %q", store.updateScope, store.updateCWD, store.updateContent)
	}
	if !reflect.DeepEqual(notices, []invalidation.Notice{{Resource: invalidation.Knowledge}}) {
		t.Fatalf("invalidations = %+v, want knowledge", notices)
	}
}

func TestRuntimeKnowledgeRejectsUnknownScopeBeforeDispatch(t *testing.T) {
	store := &fakeKnowledgeStore{}
	c := NewKnowledge(NewScope("", "", testPaths{}), knowledgeInspector{}, store, nil, nil)
	unknown := knowledge.Scope("workspace")

	if _, err := c.Read(t.Context(), unknown, "/repo"); err == nil {
		t.Fatal("Read accepted an unknown scope")
	}
	if _, err := c.Update(t.Context(), unknown, "/repo", "rev-1", "notes"); err == nil {
		t.Fatal("Update accepted an unknown scope")
	}
	if store.getScope != "" || store.updateScope != "" {
		t.Fatalf("invalid scope reached store: get=%q update=%q", store.getScope, store.updateScope)
	}
}

func TestRuntimeKnowledgeRejectsOversizedContentBeforeStore(t *testing.T) {
	store := &fakeKnowledgeStore{}
	var notices []invalidation.Notice
	c := NewKnowledge(
		NewScope("", "", testPaths{}),
		knowledgeInspector{},
		store,
		nil,
		func(notice invalidation.Notice) { notices = append(notices, notice) },
	)

	content := strings.Repeat("x", int(knowledge.MaxDocumentBytes)+1)
	if _, err := c.Update(t.Context(), knowledge.ScopeHome, "", "rev-1", content); !errors.Is(err, knowledge.ErrDocumentTooLarge) {
		t.Fatalf("Update error = %v, want ErrDocumentTooLarge", err)
	}
	if store.updateContent != "" {
		t.Fatal("oversized knowledge content reached the persistence port")
	}
	if len(notices) != 0 {
		t.Fatalf("oversized update published invalidations: %+v", notices)
	}
}

func TestRuntimeKnowledgeMapsInfraContainmentWithoutLeakingFilesystemMechanics(t *testing.T) {
	store := &fakeKnowledgeStore{err: knowledge.ErrPathOutsideScope}
	c := NewKnowledge(
		NewScope("", "", testPaths{}),
		knowledgeInspector{resolved: Resolved{Path: "/repo", ProjectRoot: "/repo"}},
		store, nil, nil,
	)

	if _, err := c.Entries(t.Context(), "/repo"); !errors.Is(err, ErrPathOutsideRoot) {
		t.Fatalf("Entries error = %v, want ErrPathOutsideRoot", err)
	}
	if _, err := c.Read(t.Context(), knowledge.ScopeHome, ""); !errors.Is(err, ErrPathOutsideRoot) {
		t.Fatalf("Read error = %v, want ErrPathOutsideRoot", err)
	}
	if _, err := c.Update(t.Context(), knowledge.ScopeHome, "", "rev-1", "notes"); !errors.Is(err, ErrPathOutsideRoot) {
		t.Fatalf("Update error = %v, want ErrPathOutsideRoot", err)
	}
}

type fakeKnowledgeStore struct {
	entries []knowledge.Entry
	entry   knowledge.Entry
	err     error

	listCWD         string
	listProjectRoot string

	getScope knowledge.Scope
	getCWD   string

	updateScope    knowledge.Scope
	updateCWD      string
	updateRevision string
	updateContent  string
}

func (s *fakeKnowledgeStore) List(_ context.Context, cwd, projectRoot string) ([]knowledge.Entry, error) {
	s.listCWD = cwd
	s.listProjectRoot = projectRoot
	return s.entries, s.err
}

func (s *fakeKnowledgeStore) Get(_ context.Context, scope knowledge.Scope, cwd string) (knowledge.Entry, error) {
	s.getScope = scope
	s.getCWD = cwd
	return s.entry, s.err
}

func (s *fakeKnowledgeStore) Update(_ context.Context, scope knowledge.Scope, cwd, expectedRevision, content string) (knowledge.Entry, error) {
	s.updateScope = scope
	s.updateCWD = cwd
	s.updateRevision = expectedRevision
	s.updateContent = content
	if s.err != nil {
		return knowledge.Entry{}, s.err
	}
	return knowledge.Entry{Scope: scope, Content: content, Revision: "rev-2"}, nil
}

type knowledgeInspector struct {
	resolved Resolved
	err      error
}

func (i knowledgeInspector) Inspect(string) (Resolved, error) {
	return i.resolved, i.err
}
