package storage_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage"
)

func TestFileMemoryService_UpdateAndGet(t *testing.T) {
	t.Setenv("LYRA_HOME", t.TempDir())

	svc, err := storage.NewFileKnowledgeStore()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	const userBody = "# User\nprefer terse output\n"
	if err = svc.Update(ctx, knowledge.ScopeUser, "", userBody); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := svc.Get(ctx, knowledge.ScopeUser, "")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != userBody {
		t.Errorf("Get returned %q, want %q", got, userBody)
	}
}

func TestFileMemoryService_GetEmptyOnFreshHome(t *testing.T) {
	t.Setenv("LYRA_HOME", t.TempDir())
	svc, _ := storage.NewFileKnowledgeStore()
	got, err := svc.Get(context.Background(), knowledge.ScopeUser, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("fresh home: want empty, got %q", got)
	}
}

func TestFileMemoryService_PersistsAcrossInstances(t *testing.T) {
	t.Setenv("LYRA_HOME", t.TempDir())

	first, _ := storage.NewFileKnowledgeStore()
	_ = first.Update(context.Background(), knowledge.ScopeUser, "", "remember me")

	second, _ := storage.NewFileKnowledgeStore()
	got, _ := second.Get(context.Background(), knowledge.ScopeUser, "")
	if got != "remember me" {
		t.Errorf("after restart got %q", got)
	}
}

func TestFileMemoryService_ConcurrentInstancesUseIndependentTemporaryFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LYRA_HOME", home)
	first, err := storage.NewFileKnowledgeStore()
	if err != nil {
		t.Fatal(err)
	}
	second, err := storage.NewFileKnowledgeStore()
	if err != nil {
		t.Fatal(err)
	}

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

func TestFileMemoryService_List_SkipsEmptyScopes(t *testing.T) {
	t.Setenv("LYRA_HOME", t.TempDir())
	svc, _ := storage.NewFileKnowledgeStore()
	ctx := context.Background()

	_ = svc.Update(ctx, knowledge.ScopeUser, "", "only user")

	entries, err := svc.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("len = %d, want 1 (project skipped)", len(entries))
	}
	if entries[0].Scope != knowledge.ScopeUser {
		t.Errorf("scope = %q, want user", entries[0].Scope)
	}
	// CapturedAt must be populated from the file mtime, not left zero (the wire
	// maps it to MemoryEntry.UpdatedAt — a zero time would surface as 0001-01-01).
	if entries[0].CapturedAt.IsZero() {
		t.Error("CapturedAt is zero; want the LYRA.md file mtime")
	}
}

func TestFileMemoryService_RejectsUnknownScope(t *testing.T) {
	t.Setenv("LYRA_HOME", t.TempDir())
	svc, err := storage.NewFileKnowledgeStore()
	if err != nil {
		t.Fatal(err)
	}
	unknown := knowledge.Scope("workspace")

	if _, err := svc.Get(t.Context(), unknown, t.TempDir()); err == nil {
		t.Fatal("Get accepted an unknown scope")
	}
	if err := svc.Update(t.Context(), unknown, t.TempDir(), "notes"); err == nil {
		t.Fatal("Update accepted an unknown scope")
	}
}

// TestFileMemoryService_ProjectScopeUsesCwd points cwd at a temp
// dir and verifies the project file ends up there (not in
// LYRA_HOME).
func TestFileMemoryService_ProjectScopeUsesCwd(t *testing.T) {
	t.Setenv("LYRA_HOME", t.TempDir())
	projectDir := t.TempDir()

	prevWd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(prevWd) })
	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}

	svc, _ := storage.NewFileKnowledgeStore()
	ctx := context.Background()
	_ = svc.Update(ctx, knowledge.ScopeProject, "", "project body")

	// File should live at <projectDir>/LYRA.md
	body, err := os.ReadFile(filepath.Join(projectDir, "LYRA.md"))
	if err != nil {
		t.Fatalf("read project file: %v", err)
	}
	if string(body) != "project body" {
		t.Errorf("project file body = %q", string(body))
	}
}

// TestFileMemoryService_ProjectScopeFollowsDir — the project scope is
// addressed by the per-call dir, so one store serves every project;
// empty dir falls back to the construction-time default.
func TestFileMemoryService_ProjectScopeFollowsDir(t *testing.T) {
	t.Setenv("LYRA_HOME", t.TempDir())
	svc, err := storage.NewFileKnowledgeStore()
	if err != nil {
		t.Fatalf("NewFileKnowledgeStore: %v", err)
	}

	dirA, dirB := t.TempDir(), t.TempDir()
	ctx := context.Background()
	if err := svc.Update(ctx, knowledge.ScopeProject, dirA, "alpha knowledge"); err != nil {
		t.Fatalf("Update dirA: %v", err)
	}

	got, err := svc.Get(ctx, knowledge.ScopeProject, dirA)
	if err != nil || got != "alpha knowledge" {
		t.Fatalf("Get dirA = (%q, %v), want alpha knowledge", got, err)
	}
	if got, _ := svc.Get(ctx, knowledge.ScopeProject, dirB); got != "" {
		t.Fatalf("Get dirB = %q, want empty (projects are isolated)", got)
	}
}
