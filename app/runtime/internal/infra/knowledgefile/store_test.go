package knowledgefile_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/advisorylock"
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
	fresh, err := store.Get(ctx, knowledge.ScopeHome, "")
	if err != nil {
		t.Fatalf("Get fresh: %v", err)
	}
	written, err := store.Update(ctx, knowledge.ScopeHome, "", fresh.Revision, userBody)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := store.Get(ctx, knowledge.ScopeHome, "")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content != userBody || got.Revision == fresh.Revision || got != written {
		t.Errorf("Get returned %+v, written %+v, fresh revision %q", got, written, fresh.Revision)
	}
}

func TestStoreGetEmptyOnFreshHome(t *testing.T) {
	store := newKnowledgeStore(t, t.TempDir(), t.TempDir())
	got, err := store.Get(context.Background(), knowledge.ScopeHome, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "" || got.Revision == "" {
		t.Errorf("fresh home: want empty content with revision, got %+v", got)
	}
}

func TestStorePersistsAcrossInstances(t *testing.T) {
	userScopeDirectory := t.TempDir()
	defaultWorkspaceDirectory := t.TempDir()
	first := newKnowledgeStore(t, userScopeDirectory, defaultWorkspaceDirectory)
	fresh, _ := first.Get(context.Background(), knowledge.ScopeHome, "")
	_, _ = first.Update(context.Background(), knowledge.ScopeHome, "", fresh.Revision, "remember me")

	second := newKnowledgeStore(t, userScopeDirectory, defaultWorkspaceDirectory)
	got, _ := second.Get(context.Background(), knowledge.ScopeHome, "")
	if got.Content != "remember me" {
		t.Errorf("after restart got %+v", got)
	}
}

func TestStoreRejectsKnowledgeSymlinkOutsideScope(t *testing.T) {
	home := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("private outside knowledge"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, "LYRA.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	store := newKnowledgeStore(t, home, t.TempDir())

	if _, err := store.Get(t.Context(), knowledge.ScopeHome, ""); !errors.Is(err, knowledge.ErrPathOutsideScope) {
		t.Fatalf("Get error = %v, want ErrPathOutsideScope", err)
	}
	if _, err := store.List(t.Context(), "", ""); !errors.Is(err, knowledge.ErrPathOutsideScope) {
		t.Fatalf("List error = %v, want ErrPathOutsideScope", err)
	}
	if _, err := store.Update(t.Context(), knowledge.ScopeHome, "", "sha256:untrusted", "overwrite"); !errors.Is(err, knowledge.ErrPathOutsideScope) {
		t.Fatalf("Update error = %v, want ErrPathOutsideScope", err)
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "private outside knowledge" {
		t.Fatalf("outside target changed to %q", got)
	}
}

func TestStoreUsesInScopeSymlinkPhysicalIdentityAndPreservesMode(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "private", "knowledge.md")
	if err := os.Mkdir(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(home, "LYRA.md")
	if err := os.Symlink(target, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	store := newKnowledgeStore(t, home, t.TempDir())

	fresh, err := store.Get(t.Context(), knowledge.ScopeHome, "")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Content != "before" {
		t.Fatalf("Get content = %q, want physical target content", fresh.Content)
	}
	written, err := store.Update(t.Context(), knowledge.ScopeHome, "", fresh.Revision, "after")
	if err != nil {
		t.Fatal(err)
	}
	if written.Content != "after" || written.Revision == fresh.Revision {
		t.Fatalf("written = %+v, fresh = %+v", written, fresh)
	}
	if info, err := os.Lstat(alias); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("knowledge alias was replaced: info=%v err=%v", info, err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "after" {
		t.Fatalf("physical target content = %q, want after", got)
	}
	if info, err := os.Stat(target); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("physical target mode = %v, err=%v, want 0600", info.Mode().Perm(), err)
	}
}

func TestStoreCreatesMissingInScopeSymlinkTarget(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "private", "knowledge.md")
	alias := filepath.Join(home, "LYRA.md")
	if err := os.Symlink(filepath.Join("private", "knowledge.md"), alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	store := newKnowledgeStore(t, home, t.TempDir())

	fresh, err := store.Get(t.Context(), knowledge.ScopeHome, "")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Content != "" || fresh.Revision == "" {
		t.Fatalf("fresh target = %+v, want addressable empty document", fresh)
	}
	if _, err := store.Update(t.Context(), knowledge.ScopeHome, "", fresh.Revision, "created"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(alias); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("knowledge alias was replaced: info=%v err=%v", info, err)
	}
	if body, err := os.ReadFile(target); err != nil || string(body) != "created" {
		t.Fatalf("target = %q, err=%v", body, err)
	}
	if info, err := os.Stat(filepath.Dir(target)); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("target directory mode = %v, err=%v, want 0700", info.Mode().Perm(), err)
	}
}

func TestStoreUpdatePreservesRegularFileMode(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "LYRA.md")
	if err := os.WriteFile(path, []byte("before"), 0o640); err != nil {
		t.Fatal(err)
	}
	store := newKnowledgeStore(t, home, t.TempDir())
	fresh, err := store.Get(t.Context(), knowledge.ScopeHome, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(t.Context(), knowledge.ScopeHome, "", fresh.Revision, "after"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v, want 0640", info.Mode().Perm())
	}
}

func TestStoreUsesPrivateModeForNewHomeAndReadableModeForNewWorkspace(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	store := newKnowledgeStore(t, home, workspace)
	for _, scope := range []struct {
		value knowledge.Scope
		dir   string
		mode  os.FileMode
		path  string
	}{
		{value: knowledge.ScopeHome, mode: 0o600, path: filepath.Join(home, "LYRA.md")},
		{value: knowledge.ScopeCWD, dir: workspace, mode: 0o644, path: filepath.Join(workspace, "LYRA.md")},
	} {
		fresh, err := store.Get(t.Context(), scope.value, scope.dir)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Update(t.Context(), scope.value, scope.dir, fresh.Revision, "created"); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(scope.path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != scope.mode {
			t.Errorf("%s mode = %v, want %v", scope.value, info.Mode().Perm(), scope.mode)
		}
	}
}

func TestStoreConcurrentUpdatesRejectStaleRevisionsWithoutTornWrites(t *testing.T) {
	home := t.TempDir()
	defaultWorkspaceDirectory := t.TempDir()
	store := newKnowledgeStore(t, home, defaultWorkspaceDirectory)
	fresh, err := store.Get(t.Context(), knowledge.ScopeHome, "")
	if err != nil {
		t.Fatal(err)
	}

	// A fixed sibling used by an older implementation must not be a reserved path.
	legacyTemporary := filepath.Join(home, "LYRA.md.tmp")
	if err := os.Mkdir(legacyTemporary, 0o755); err != nil {
		t.Fatal(err)
	}

	wantBodies := make(map[string]struct{}, 32)
	written := make(chan knowledge.Entry, 32)
	writeErrors := make(chan error, 32)
	var writes sync.WaitGroup
	for index := range 32 {
		body := fmt.Sprintf("knowledge from writer %02d", index)
		wantBodies[body] = struct{}{}
		writes.Go(func() {
			entry, err := store.Update(
				t.Context(), knowledge.ScopeHome, "", fresh.Revision, body,
			)
			if err != nil {
				writeErrors <- err
				return
			}
			written <- entry
		})
	}
	writes.Wait()
	close(written)
	close(writeErrors)
	if len(written) != 1 {
		t.Fatalf("successful writes = %d, want exactly one CAS winner", len(written))
	}
	if len(writeErrors) != 31 {
		t.Fatalf("failed writes = %d, want 31 stale revisions", len(writeErrors))
	}
	for err := range writeErrors {
		if !errors.Is(err, knowledge.ErrRevisionConflict) {
			t.Errorf("concurrent Update: %v, want revision conflict", err)
		}
	}
	if t.Failed() {
		return
	}
	body, err := store.Get(t.Context(), knowledge.ScopeHome, "")
	if err != nil {
		t.Fatalf("read final knowledge: %v", err)
	}
	if _, ok := wantBodies[body.Content]; !ok {
		t.Fatalf("final knowledge = %q, want one complete writer value", body.Content)
	}
	if info, err := os.Stat(legacyTemporary); err != nil || !info.IsDir() {
		t.Fatalf("legacy temporary path changed: info=%v err=%v", info, err)
	}
}

func TestStoreCrossProcessUpdatesHaveOneCASWinner(t *testing.T) {
	if os.Getenv("LYRA_TEST_KNOWLEDGE_CAS_CHILD") == "1" {
		store := newKnowledgeStore(t, os.Getenv("LYRA_TEST_KNOWLEDGE_HOME"), t.TempDir())
		ready := os.Getenv("LYRA_TEST_KNOWLEDGE_READY")
		start := os.Getenv("LYRA_TEST_KNOWLEDGE_START")
		if err := os.WriteFile(ready, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		for {
			if _, err := os.Stat(start); err == nil {
				break
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			time.Sleep(time.Millisecond)
		}
		_, err := store.Update(
			t.Context(), knowledge.ScopeHome, "",
			os.Getenv("LYRA_TEST_KNOWLEDGE_REVISION"), os.Getenv("LYRA_TEST_KNOWLEDGE_BODY"),
		)
		switch {
		case err == nil:
			_, _ = os.Stdout.WriteString("written")
		case errors.Is(err, knowledge.ErrRevisionConflict):
			_, _ = os.Stdout.WriteString("conflict")
		default:
			t.Fatal(err)
		}
		os.Exit(0)
	}

	home := t.TempDir()
	store := newKnowledgeStore(t, home, t.TempDir())
	fresh, err := store.Get(t.Context(), knowledge.ScopeHome, "")
	if err != nil {
		t.Fatal(err)
	}
	barrier := t.TempDir()
	start := filepath.Join(barrier, "start")
	type child struct {
		command *exec.Cmd
		output  strings.Builder
	}
	children := make([]child, 12)
	for index := range children {
		ready := filepath.Join(barrier, fmt.Sprintf("ready-%02d", index))
		children[index].command = exec.Command(os.Args[0], "-test.run=^TestStoreCrossProcessUpdatesHaveOneCASWinner$")
		children[index].command.Env = append(os.Environ(),
			"LYRA_TEST_KNOWLEDGE_CAS_CHILD=1",
			"LYRA_TEST_KNOWLEDGE_HOME="+home,
			"LYRA_TEST_KNOWLEDGE_READY="+ready,
			"LYRA_TEST_KNOWLEDGE_START="+start,
			"LYRA_TEST_KNOWLEDGE_REVISION="+fresh.Revision,
			fmt.Sprintf("LYRA_TEST_KNOWLEDGE_BODY=writer-%02d", index),
		)
		children[index].command.Stdout = &children[index].output
		children[index].command.Stderr = &children[index].output
		if err := children[index].command.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = children[index].command.Process.Kill() })
	}
	for index := range children {
		ready := filepath.Join(barrier, fmt.Sprintf("ready-%02d", index))
		for {
			if _, err := os.Stat(ready); err == nil {
				break
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			time.Sleep(time.Millisecond)
		}
	}
	if err := os.WriteFile(start, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	winners := 0
	for index := range children {
		if err := children[index].command.Wait(); err != nil {
			t.Fatalf("child %d: %v\n%s", index, err, children[index].output.String())
		}
		switch output := children[index].output.String(); {
		case strings.Contains(output, "written"):
			winners++
		case strings.Contains(output, "conflict"):
		default:
			t.Fatalf("child %d result = %q", index, output)
		}
	}
	if winners != 1 {
		t.Fatalf("cross-process CAS winners = %d, want exactly one", winners)
	}
}

func TestStoreRecoversAfterWriterProcessDiesDuringStaging(t *testing.T) {
	if os.Getenv("LYRA_TEST_KNOWLEDGE_CRASH_CHILD") == "1" {
		store := newKnowledgeStore(t, os.Getenv("LYRA_TEST_KNOWLEDGE_HOME"), t.TempDir())
		body := strings.Repeat("x", 64<<20)
		if err := os.WriteFile(os.Getenv("LYRA_TEST_KNOWLEDGE_READY"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := store.Update(
			t.Context(), knowledge.ScopeHome, "",
			os.Getenv("LYRA_TEST_KNOWLEDGE_REVISION"), body,
		)
		if err != nil {
			t.Fatal(err)
		}
		os.Exit(0)
	}

	home := t.TempDir()
	store := newKnowledgeStore(t, home, t.TempDir())
	fresh, err := store.Get(t.Context(), knowledge.ScopeHome, "")
	if err != nil {
		t.Fatal(err)
	}
	written, err := store.Update(t.Context(), knowledge.ScopeHome, "", fresh.Revision, "before")
	if err != nil {
		t.Fatal(err)
	}
	lease, err := advisorylock.AcquireDirectory(t.Context(), home)
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(t.TempDir(), "ready")
	command := exec.Command(os.Args[0], "-test.run=^TestStoreRecoversAfterWriterProcessDiesDuringStaging$")
	command.Env = append(os.Environ(),
		"LYRA_TEST_KNOWLEDGE_CRASH_CHILD=1",
		"LYRA_TEST_KNOWLEDGE_HOME="+home,
		"LYRA_TEST_KNOWLEDGE_READY="+ready,
		"LYRA_TEST_KNOWLEDGE_REVISION="+written.Revision,
	)
	var output strings.Builder
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		_ = lease.Release()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill() })
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			_ = lease.Release()
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(10 * time.Second)
	staged := ""
	for staged == "" && time.Now().Before(deadline) {
		entries, err := os.ReadDir(home)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".LYRA.md.lyra-stage-") {
				staged = filepath.Join(home, entry.Name())
				break
			}
		}
	}
	if staged == "" {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("writer produced no observable staging file: %s", output.String())
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()

	restarted := newKnowledgeStore(t, home, t.TempDir())
	afterCrash, err := restarted.Get(t.Context(), knowledge.ScopeHome, "")
	if err != nil {
		t.Fatal(err)
	}
	if afterCrash.Content != "before" || afterCrash.Revision != written.Revision {
		t.Fatalf("after crash = %+v, want the pre-crash committed document", afterCrash)
	}
	if _, err := os.Stat(staged); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cold read did not remove orphan staging file: %v", err)
	}
	recovered, err := restarted.Update(t.Context(), knowledge.ScopeHome, "", written.Revision, "after")
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Content != "after" {
		t.Fatalf("recovered = %+v", recovered)
	}
}

func TestStoreListIncludesEmptyAddressableScopes(t *testing.T) {
	store := newKnowledgeStore(t, t.TempDir(), t.TempDir())
	ctx := context.Background()

	fresh, _ := store.Get(ctx, knowledge.ScopeHome, "")
	_, _ = store.Update(ctx, knowledge.ScopeHome, "", fresh.Revision, "only user")

	entries, err := store.List(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("len = %d, want home plus the de-duplicated workspace document", len(entries))
	}
	if entries[0].Scope != knowledge.ScopeHome || entries[0].Revision == "" || entries[1].Content != "" || entries[1].Revision == "" {
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
		fresh, err := store.Get(t.Context(), write.scope, write.dir)
		if err != nil {
			t.Fatalf("Get(%s): %v", write.scope, err)
		}
		if _, err := store.Update(t.Context(), write.scope, write.dir, fresh.Revision, write.body); err != nil {
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
		physicalPath, err := filepath.EvalSymlinks(want.path)
		if err != nil {
			t.Fatal(err)
		}
		if got.Scope != want.scope || got.Content != want.body || got.Path != physicalPath || got.UpdatedAt.IsZero() {
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
	if _, err := store.Update(t.Context(), unknown, t.TempDir(), "revision", "notes"); err == nil {
		t.Fatal("Update accepted an unknown scope")
	}
}

// TestStoreProjectScopeUsesConfiguredDefault proves the adapter
// uses its explicit fallback and never reads the process working directory.
func TestStoreProjectScopeUsesConfiguredDefault(t *testing.T) {
	projectDir := t.TempDir()
	store := newKnowledgeStore(t, t.TempDir(), projectDir)
	ctx := context.Background()
	fresh, _ := store.Get(ctx, knowledge.ScopeCWD, "")
	_, _ = store.Update(ctx, knowledge.ScopeCWD, "", fresh.Revision, "project body")

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
	fresh, _ := store.Get(ctx, knowledge.ScopeCWD, dirA)
	if _, err := store.Update(ctx, knowledge.ScopeCWD, dirA, fresh.Revision, "alpha knowledge"); err != nil {
		t.Fatalf("Update dirA: %v", err)
	}

	got, err := store.Get(ctx, knowledge.ScopeCWD, dirA)
	if err != nil || got.Content != "alpha knowledge" {
		t.Fatalf("Get dirA = (%+v, %v), want alpha knowledge", got, err)
	}
	if got, _ := store.Get(ctx, knowledge.ScopeCWD, dirB); got.Content != "" {
		t.Fatalf("Get dirB = %+v, want empty (projects are isolated)", got)
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
