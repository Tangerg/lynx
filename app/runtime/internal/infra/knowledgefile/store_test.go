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

func newKnowledgeStore(t *testing.T, userScopeDirectory, defaultWorkspaceDirectory string) *knowledgefile.Store {
	t.Helper()
	store, err := knowledgefile.New(userScopeDirectory, defaultWorkspaceDirectory)
	if err != nil {
		t.Fatalf("knowledgefile.New: %v", err)
	}
	return store
}

func TestStoreUpdateAndGet(t *testing.T) {
	store := newKnowledgeStore(t, t.TempDir(), t.TempDir())
	ctx := context.Background()

	const userBody = "# User\nprefer terse output\n"
	if err := store.Update(ctx, knowledge.ScopeHome, "", userBody); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := store.Get(ctx, knowledge.ScopeHome, "")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != userBody {
		t.Errorf("Get returned %q, want %q", got, userBody)
	}
}

func TestStoreGetEmptyOnFreshHome(t *testing.T) {
	store := newKnowledgeStore(t, t.TempDir(), t.TempDir())
	got, err := store.Get(context.Background(), knowledge.ScopeHome, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("fresh home: want empty, got %q", got)
	}
}

func TestStorePersistsAcrossInstances(t *testing.T) {
	userScopeDirectory := t.TempDir()
	defaultWorkspaceDirectory := t.TempDir()
	first := newKnowledgeStore(t, userScopeDirectory, defaultWorkspaceDirectory)
	_ = first.Update(context.Background(), knowledge.ScopeHome, "", "remember me")

	second := newKnowledgeStore(t, userScopeDirectory, defaultWorkspaceDirectory)
	got, _ := second.Get(context.Background(), knowledge.ScopeHome, "")
	if got != "remember me" {
		t.Errorf("after restart got %q", got)
	}
}

func TestStoreConcurrentInstancesUseIndependentTemporaryFiles(t *testing.T) {
	home := t.TempDir()
	defaultWorkspaceDirectory := t.TempDir()
	first := newKnowledgeStore(t, home, defaultWorkspaceDirectory)
	second := newKnowledgeStore(t, home, defaultWorkspaceDirectory)

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
			if err := store.Update(t.Context(), knowledge.ScopeHome, "", body); err != nil {
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
	body, err := first.Get(t.Context(), knowledge.ScopeHome, "")
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

	_ = store.Update(ctx, knowledge.ScopeHome, "", "only user")

	entries, err := store.List(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("len = %d, want 1 (project skipped)", len(entries))
	}
	if entries[0].Scope != knowledge.ScopeHome {
		t.Errorf("scope = %q, want user", entries[0].Scope)
	}
	// UpdatedAt must be populated from the file mtime, not left zero (the wire
	// maps it to KnowledgeEntry.UpdatedAt — a zero time would surface as 0001-01-01).
	if entries[0].UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero; want the LYRA.md file mtime")
	}
}

func TestStoreListPreservesDistinctCascadeScopes(t *testing.T) {
	home := t.TempDir()
	projectRoot := t.TempDir()
	cwd := filepath.Join(projectRoot, "packages", "desktop")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	store := newKnowledgeStore(t, home, cwd)
	for _, write := range []struct {
		scope knowledge.Scope
		dir   string
		body  string
	}{
		{knowledge.ScopeHome, "", "home knowledge"},
		{knowledge.ScopeProjectRoot, projectRoot, "project knowledge"},
		{knowledge.ScopeCWD, cwd, "workspace knowledge"},
	} {
		if err := store.Update(t.Context(), write.scope, write.dir, write.body); err != nil {
			t.Fatalf("Update(%s): %v", write.scope, err)
		}
	}

	entries, err := store.List(t.Context(), cwd, projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %+v, want three cascade locations", entries)
	}
	for index, want := range []struct {
		scope knowledge.Scope
		body  string
		path  string
	}{
		{knowledge.ScopeHome, "home knowledge", filepath.Join(home, "LYRA.md")},
		{knowledge.ScopeProjectRoot, "project knowledge", filepath.Join(projectRoot, "LYRA.md")},
		{knowledge.ScopeCWD, "workspace knowledge", filepath.Join(cwd, "LYRA.md")},
	} {
		got := entries[index]
		if got.Scope != want.scope || got.Content != want.body || got.Path != want.path || got.UpdatedAt.IsZero() {
			t.Errorf("entries[%d] = %+v, want scope=%s body=%q path=%q with mtime", index, got, want.scope, want.body, want.path)
		}
	}

	rootEntries, err := store.List(t.Context(), projectRoot, projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(rootEntries) != 2 || rootEntries[1].Scope != knowledge.ScopeCWD {
		t.Fatalf("root entries = %+v, want home plus one cwd document", rootEntries)
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
	_ = store.Update(ctx, knowledge.ScopeCWD, "", "project body")

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
	if err := store.Update(ctx, knowledge.ScopeCWD, dirA, "alpha knowledge"); err != nil {
		t.Fatalf("Update dirA: %v", err)
	}

	got, err := store.Get(ctx, knowledge.ScopeCWD, dirA)
	if err != nil || got != "alpha knowledge" {
		t.Fatalf("Get dirA = (%q, %v), want alpha knowledge", got, err)
	}
	if got, _ := store.Get(ctx, knowledge.ScopeCWD, dirB); got != "" {
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
