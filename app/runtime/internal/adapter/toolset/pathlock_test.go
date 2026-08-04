package toolset

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/tools/fs"
)

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
	tracker := newReadTracker()
	read := withPathLock(withReadTracking(fs.NewReadTool(executor), tracker, workdir), locker, workdir)
	edit := editMutation(fs.NewEditTool(executor), nil, tracker, locker, workdir)

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
	tracker := newReadTracker()
	read := withPathLock(withReadTracking(fs.NewReadTool(executor), tracker, workdir), locker, workdir)
	write := writeMutation(fs.NewWriteTool(executor), nil, tracker, locker, workdir)
	realKey := concurrentKey(t, read, pathArguments(realPath))
	aliasKey := concurrentKey(t, write, pathArguments(aliasPath))
	if realKey != aliasKey {
		t.Fatalf("symlink alias keys = %q, %q; want one physical identity", realKey, aliasKey)
	}
}

func TestPathLockKeepsMultiFilePatchExclusive(t *testing.T) {
	workdir := t.TempDir()
	tool := withPathLock(fs.NewApplyPatchTool(fs.NewLocalExecutor(workdir)), newPathLocker(), workdir)
	policy, ok, err := toolcontract.Capability[toolloop.ConcurrentTool](tool)
	if err != nil || !ok {
		t.Fatal("path-locked apply_patch does not expose concurrency policy")
	}
	key, concurrent := policy.ConcurrencyKey(`{"patch":"--- a/one.txt\n+++ b/one.txt\n--- a/two.txt\n+++ b/two.txt\n"}`)
	if key != "" || concurrent {
		t.Fatalf("apply_patch concurrency = %q, %v; want exclusive", key, concurrent)
	}
}

// TestAssembledFileToolStillReportsWhatItMutates pins the wrapping chain
// through the real stack. A guarded mutation tool is six wrappers deep, and
// everything above it asks the OUTERMOST tool what the call will touch — the
// approval gate renders that blast radius, and the tool-end event reports the
// paths that changed. A layer that stops being a WrappingTool ends the chain and
// silently answers "nothing", which reads as a safe tool rather than a broken
// lookup.
func TestAssembledFileToolStillReportsWhatItMutates(t *testing.T) {
	workdir := t.TempDir()
	target := filepath.Join(workdir, "real.txt")
	if err := os.WriteFile(target, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	assembled := editMutation(
		fs.NewEditTool(fs.NewLocalExecutor(workdir)),
		nil,
		newReadTracker(),
		newPathLocker(),
		workdir,
	)

	reporter, ok, err := toolcontract.Capability[fileMutationReporter](assembled)
	if err != nil || !ok {
		t.Fatal("the assembled edit tool no longer reports its file mutations")
	}
	paths, err := reporter.MutationPaths(pathArguments("real.txt"))
	if err != nil {
		t.Fatalf("MutationPaths: %v", err)
	}
	if len(paths) != 1 || filepath.Base(paths[0]) != "real.txt" {
		t.Fatalf("MutationPaths = %v, want the edited file", paths)
	}
}

func concurrentKey(t *testing.T, tool toolcontract.Tool, arguments string) string {
	t.Helper()
	policy, ok, err := toolcontract.Capability[toolloop.ConcurrentTool](tool)
	if err != nil || !ok {
		t.Fatalf("tool %q does not expose concurrency policy", tool.Definition().Name)
	}
	key, concurrent := policy.ConcurrencyKey(arguments)
	if !concurrent {
		t.Fatalf("tool %q unexpectedly remained exclusive", tool.Definition().Name)
	}
	return key
}

func pathArguments(path string) string {
	return `{"path":` + strconv.Quote(path) + `}`
}
