package knowledgefile_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/knowledgefile"
)

func newKnowledgeStore(t *testing.T, userScopeDirectory, defaultProjectDirectory string) *knowledgefile.Store {
	t.Helper()
	store, err := knowledgefile.New(userScopeDirectory, defaultProjectDirectory)
	if err != nil {
		t.Fatalf("knowledgefile.New: %v", err)
	}
	return store
}

func TestStoreUpdateAndGet(t *testing.T) {
	store := newKnowledgeStore(t, t.TempDir(), t.TempDir())
	ctx := context.Background()

	const userBody = "# User\nprefer terse output\n"
	if err := store.Update(ctx, knowledge.ScopeUser, "", userBody); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := store.Get(ctx, knowledge.ScopeUser, "")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != userBody {
		t.Errorf("Get returned %q, want %q", got, userBody)
	}
}

func TestStoreGetEmptyOnFreshHome(t *testing.T) {
	store := newKnowledgeStore(t, t.TempDir(), t.TempDir())
	got, err := store.Get(context.Background(), knowledge.ScopeUser, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("fresh home: want empty, got %q", got)
	}
}

func TestStorePersistsAcrossInstances(t *testing.T) {
	userScopeDirectory := t.TempDir()
	defaultProjectDirectory := t.TempDir()
	first := newKnowledgeStore(t, userScopeDirectory, defaultProjectDirectory)
	_ = first.Update(context.Background(), knowledge.ScopeUser, "", "remember me")

	second := newKnowledgeStore(t, userScopeDirectory, defaultProjectDirectory)
	got, _ := second.Get(context.Background(), knowledge.ScopeUser, "")
	if got != "remember me" {
		t.Errorf("after restart got %q", got)
	}
}

func TestStoreConcurrentInstancesUseIndependentTemporaryFiles(t *testing.T) {
	home := t.TempDir()
	defaultProjectDirectory := t.TempDir()
	first := newKnowledgeStore(t, home, defaultProjectDirectory)
	second := newKnowledgeStore(t, home, defaultProjectDirectory)

	// A fixed sibling used by an older implementation must not be a reserved path.
	legacyTemporary := filepath.Join(home, "LYRA.md.tmp")
	if err := os.Mkdir(legacyTemporary, 0o755); err != nil {
		t.Fatal(err)
	}

	wantBodies := make(map[string]struct{}, 32)
	writeErrors := make(chan error, 32)
	var writes sync.WaitGroup
	for index := range 32 {
		store := first
		if index%2 != 0 {
			store = second
		}
		body := fmt.Sprintf("knowledge from writer %02d", index)
		wantBodies[body] = struct{}{}
		writes.Go(func() {
			if err := store.Update(t.Context(), knowledge.ScopeUser, "", body); err != nil {
				writeErrors <- err
			}
		})
	}
	writes.Wait()
	close(writeErrors)
	for err := range writeErrors {
		t.Errorf("concurrent Update: %v", err)
	}
	if t.Failed() {
		return
	}
	body, err := first.Get(t.Context(), knowledge.ScopeUser, "")
	if err != nil {
		t.Fatalf("read final knowledge: %v", err)
	}
	if _, ok := wantBodies[body]; !ok {
		t.Fatalf("final knowledge = %q, want one complete writer value", body)
	}
	if info, err := os.Stat(legacyTemporary); err != nil || !info.IsDir() {
		t.Fatalf("legacy temporary path changed: info=%v err=%v", info, err)
	}
}

func TestStoreList_SkipsEmptyScopes(t *testing.T) {
	store := newKnowledgeStore(t, t.TempDir(), t.TempDir())
	ctx := context.Background()

	_ = store.Update(ctx, knowledge.ScopeUser, "", "only user")

	entries, err := store.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("len = %d, want 1 (project skipped)", len(entries))
	}
	if entries[0].Scope != knowledge.ScopeUser {
		t.Errorf("scope = %q, want user", entries[0].Scope)
	}
	// UpdatedAt must be populated from the file mtime, not left zero (the wire
	// maps it to KnowledgeEntry.UpdatedAt — a zero time would surface as 0001-01-01).
	if entries[0].UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero; want the LYRA.md file mtime")
	}
}

func TestStoreRejectsUnknownScope(t *testing.T) {
	store := newKnowledgeStore(t, t.TempDir(), t.TempDir())
	unknown := knowledge.Scope("workspace")

	if _, err := store.Get(t.Context(), unknown, t.TempDir()); err == nil {
		t.Fatal("Get accepted an unknown scope")
	}
	if err := store.Update(t.Context(), unknown, t.TempDir(), "notes"); err == nil {
		t.Fatal("Update accepted an unknown scope")
	}
}

// TestStoreProjectScopeUsesConfiguredDefault proves the adapter
// uses its explicit fallback and never reads the process working directory.
func TestStoreProjectScopeUsesConfiguredDefault(t *testing.T) {
	projectDir := t.TempDir()
	store := newKnowledgeStore(t, t.TempDir(), projectDir)
	ctx := context.Background()
	_ = store.Update(ctx, knowledge.ScopeProject, "", "project body")

	// File should live at <projectDir>/LYRA.md
	body, err := os.ReadFile(filepath.Join(projectDir, "LYRA.md"))
	if err != nil {
		t.Fatalf("read project file: %v", err)
	}
	if string(body) != "project body" {
		t.Errorf("project file body = %q", string(body))
	}
}

// TestStoreProjectScopeFollowsDir — the project scope is
// addressed by the per-call dir, so one store serves every project;
// empty dir falls back to the construction-time default.
func TestStoreProjectScopeFollowsDir(t *testing.T) {
	store := newKnowledgeStore(t, t.TempDir(), t.TempDir())

	dirA, dirB := t.TempDir(), t.TempDir()
	ctx := context.Background()
	if err := store.Update(ctx, knowledge.ScopeProject, dirA, "alpha knowledge"); err != nil {
		t.Fatalf("Update dirA: %v", err)
	}

	got, err := store.Get(ctx, knowledge.ScopeProject, dirA)
	if err != nil || got != "alpha knowledge" {
		t.Fatalf("Get dirA = (%q, %v), want alpha knowledge", got, err)
	}
	if got, _ := store.Get(ctx, knowledge.ScopeProject, dirB); got != "" {
		t.Fatalf("Get dirB = %q, want empty (projects are isolated)", got)
	}
}

func TestNewRequiresBothCompositionPaths(t *testing.T) {
	if _, err := knowledgefile.New("", t.TempDir()); err == nil {
		t.Fatal("New accepted an empty user scope directory")
	}
	if _, err := knowledgefile.New(t.TempDir(), ""); err == nil {
		t.Fatal("New accepted an empty default project directory")
	}
	if _, err := knowledgefile.New("relative-user", t.TempDir()); err == nil {
		t.Fatal("New accepted a relative user scope directory")
	}
	if _, err := knowledgefile.New(t.TempDir(), "relative-project"); err == nil {
		t.Fatal("New accepted a relative default project directory")
	}
}
