package toolset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/scope/tools/fs"
)

func TestPathLockUsesOneCanonicalMutationIdentity(t *testing.T) {
	cwd := t.TempDir()
	realPath := filepath.Join(cwd, "real.txt")
	if err := os.WriteFile(realPath, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "other.txt"), []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}

	executor := fs.NewLocalExecutor(cwd)
	readPaths, err := resolvedMutationPaths(fs.NewReadTool(executor), readArguments(realPath), cwd)
	if err != nil {
		t.Fatal(err)
	}
	mutationPaths, err := resolvedMutationPaths(fs.NewApplyPatchTool(executor), patchArguments(t, "real.txt", "content", "next"), cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(readPaths) != 1 || len(mutationPaths) != 1 || readPaths[0] != mutationPaths[0] {
		t.Fatalf("same-file identities = %v, %v; want one canonical path", readPaths, mutationPaths)
	}
}

func TestPathLockUsesPhysicalIdentityForSymlinkAlias(t *testing.T) {
	cwd := t.TempDir()
	realPath := filepath.Join(cwd, "real.txt")
	if err := os.WriteFile(realPath, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	aliasPath := filepath.Join(cwd, "alias.txt")
	if err := os.Symlink("real.txt", aliasPath); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	executor := fs.NewLocalExecutor(cwd)
	realPaths, err := resolvedMutationPaths(fs.NewReadTool(executor), readArguments(realPath), cwd)
	if err != nil {
		t.Fatal(err)
	}
	aliasPaths, err := resolvedMutationPaths(fs.NewApplyPatchTool(executor), patchArguments(t, "alias.txt", "content", "next"), cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(realPaths) != 1 || len(aliasPaths) != 1 || realPaths[0] != aliasPaths[0] {
		t.Fatalf("symlink alias identities = %v, %v; want one physical path", realPaths, aliasPaths)
	}
}

func TestPathLockKeepsMultiFilePatchExclusive(t *testing.T) {
	cwd := t.TempDir()
	tool := withPathLock(fs.NewApplyPatchTool(fs.NewLocalExecutor(cwd)), newPathLocker(), cwd)
	policy, ok, err := toolcontract.Capability[concurrentTool](tool)
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
	cwd := t.TempDir()
	target := filepath.Join(cwd, "real.txt")
	if err := os.WriteFile(target, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	assembled := guardedMutation(
		fs.NewApplyPatchTool(fs.NewLocalExecutor(cwd)),
		nil,
		newReadTracker(),
		newPathLocker(),
		cwd,
	)

	reporter, ok, err := toolcontract.Capability[fileMutationReporter](assembled)
	if err != nil || !ok {
		t.Fatal("the assembled mutation tool no longer reports its file mutations")
	}
	paths, err := reporter.MutationPaths(patchArguments(t, "real.txt", "content", "next"))
	if err != nil {
		t.Fatalf("MutationPaths: %v", err)
	}
	if len(paths) != 1 || filepath.Base(paths[0]) != "real.txt" {
		t.Fatalf("MutationPaths = %v, want the patched file", paths)
	}
}

func readArguments(path string) string {
	encoded, _ := json.Marshal(map[string]string{"path": path})
	return string(encoded)
}
