package agentmemory

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

type fakeStore struct {
	listScope   domain.Scope
	listProject string
	updatedAt   time.Time
	content     *string
	pinned      *bool
}

func (s *fakeStore) List(_ context.Context, scope domain.Scope, project string) ([]domain.Item, error) {
	s.listScope, s.listProject = scope, project
	return []domain.Item{{ID: "mem_1", Scope: scope, Project: project}}, nil
}

func (*fakeStore) SetStatus(context.Context, string, domain.Status, time.Time) error { return nil }

func (s *fakeStore) Update(_ context.Context, _ string, content *string, pinned *bool, now time.Time) (domain.Item, error) {
	s.content, s.pinned, s.updatedAt = content, pinned, now
	return domain.Item{ID: "mem_1"}, nil
}

func (*fakeStore) Delete(context.Context, string) error { return nil }

func (*fakeStore) Add(context.Context, domain.Scope, string, string, time.Time) (domain.Item, error) {
	return domain.Item{}, nil
}

type rootResolver struct {
	root string
	err  error
}

func (r rootResolver) ResolveRoot(string) (string, error) { return r.root, r.err }

func TestListResolvesProjectAtApplicationBoundary(t *testing.T) {
	store := &fakeStore{}
	c := New(Config{Store: store, Roots: rootResolver{root: "/canonical/repo"}})

	items, err := c.List(context.Background(), domain.ScopeProject, "/repo/../repo")
	if err != nil || len(items) != 1 {
		t.Fatalf("List = (%+v, %v)", items, err)
	}
	if store.listScope != domain.ScopeProject || store.listProject != "/canonical/repo" {
		t.Fatalf("store target = %v %q", store.listScope, store.listProject)
	}

	if _, err := c.List(context.Background(), domain.ScopeUser, "/ignored"); err != nil {
		t.Fatal(err)
	}
	if store.listScope != domain.ScopeUser || store.listProject != "" {
		t.Fatalf("user target = %v %q", store.listScope, store.listProject)
	}
}

func TestUpdateDelegatesOneAtomicPatchWithApplicationClock(t *testing.T) {
	store := &fakeStore{}
	now := time.Date(2026, 7, 23, 9, 0, 0, 0, time.UTC)
	c := New(Config{Store: store, Now: func() time.Time { return now }})
	content := "- use table-driven tests"
	pinned := true

	if _, err := c.Update(context.Background(), "mem_1", &content, &pinned); err != nil {
		t.Fatal(err)
	}
	if store.content != &content || store.pinned != &pinned || !store.updatedAt.Equal(now) {
		t.Fatalf("patch = content=%p pinned=%p at=%s", store.content, store.pinned, store.updatedAt)
	}
}

func TestDisabledCoordinatorFailsExplicitly(t *testing.T) {
	c := New(Config{})
	if _, err := c.List(context.Background(), domain.ScopeProject, "/repo"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("List error = %v, want ErrUnavailable", err)
	}
}
