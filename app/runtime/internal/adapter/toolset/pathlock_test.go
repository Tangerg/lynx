package toolset

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/editguardstate"
	"github.com/Tangerg/lynx/tools"
	"github.com/Tangerg/lynx/tools/fs"
)

type concurrencyKeyer interface {
	ConcurrencyKey(arguments string) (key string, concurrent bool)
}

func TestPathLockUsesCanonicalConcurrencyKey(t *testing.T) {
	workdir := t.TempDir()
	realPath := filepath.Join(workdir, "real.txt")
	if err := os.WriteFile(realPath, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workdir, "other.txt"), []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}

	executor := fs.NewLocalExecutor(workdir)
	locker := newPathLocker()
	tracker := editguardstate.NewTracker()
	read := withPathLock(withReadTracking(fs.NewReadTool(executor), tracker, workdir), locker, workdir)
	edit := editMutationTool(fs.NewEditTool(executor), nil, tracker, locker, workdir)

	relativeKey := concurrentKey(t, read, pathArguments("real.txt"))
	absoluteKey := concurrentKey(t, edit, pathArguments(realPath))
	if relativeKey != absoluteKey {
		t.Fatalf("same-file keys = %q, %q; want one canonical identity", relativeKey, absoluteKey)
	}
	if !strings.HasPrefix(relativeKey, fileResourceKeyPrefix) {
		t.Fatalf("canonical key = %q, want %q prefix", relativeKey, fileResourceKeyPrefix)
	}

	otherKey := concurrentKey(t, read, pathArguments("other.txt"))
	if otherKey == relativeKey {
		t.Fatalf("distinct files share concurrency key %q", otherKey)
	}
}

func TestPathLockUsesPhysicalIdentityForSymlinkAlias(t *testing.T) {
	workdir := t.TempDir()
	realPath := filepath.Join(workdir, "real.txt")
	if err := os.WriteFile(realPath, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	aliasPath := filepath.Join(workdir, "alias.txt")
	if err := os.Symlink("real.txt", aliasPath); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	executor := fs.NewLocalExecutor(workdir)
	locker := newPathLocker()
	tracker := editguardstate.NewTracker()
	read := withPathLock(withReadTracking(fs.NewReadTool(executor), tracker, workdir), locker, workdir)
	write := writeMutationTool(fs.NewWriteTool(executor), nil, tracker, locker, workdir)
	realKey := concurrentKey(t, read, pathArguments(realPath))
	aliasKey := concurrentKey(t, write, pathArguments(aliasPath))
	if realKey != aliasKey {
		t.Fatalf("symlink alias keys = %q, %q; want one physical identity", realKey, aliasKey)
	}
}

func TestPathLockKeepsMultiFilePatchExclusive(t *testing.T) {
	workdir := t.TempDir()
	tool := withPathLock(fs.NewApplyPatchTool(fs.NewLocalExecutor(workdir)), newPathLocker(), workdir)
	policy, ok := tool.(concurrencyKeyer)
	if !ok {
		t.Fatal("path-locked apply_patch does not expose concurrency policy")
	}
	key, concurrent := policy.ConcurrencyKey(`{"patch":"--- a/one.txt\n+++ b/one.txt\n--- a/two.txt\n+++ b/two.txt\n"}`)
	if key != "" || concurrent {
		t.Fatalf("apply_patch concurrency = %q, %v; want exclusive", key, concurrent)
	}
}

func concurrentKey(t *testing.T, tool tools.Tool, arguments string) string {
	t.Helper()
	policy, ok := tool.(concurrencyKeyer)
	if !ok {
		t.Fatalf("tool %q does not expose concurrency policy", tool.Definition().Name)
	}
	key, concurrent := policy.ConcurrencyKey(arguments)
	if !concurrent {
		t.Fatalf("tool %q unexpectedly remained exclusive", tool.Definition().Name)
	}
	return key
}

func pathArguments(path string) string {
	return `{"file_path":` + strconv.Quote(path) + `}`
}
