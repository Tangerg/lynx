package toolset

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/component/pathidentity"
)

// protectedDirs are directory names the agent must never modify, even when
// they sit inside the workspace. Mutating .git (a hook, the
// config) is a remote-code-execution / repo-hijack vector, so the VCS
// metadata directory is carved read-only regardless of approval mode — the
// standard invariant enforced on writable roots. A model that needs
// to change version-control state uses the shell/git tooling, not a raw
// file mutation. Kept as a list so other state dirs can join if needed.
var protectedDirs = []string{".git"}

// withPathGuard wraps a file-mutating tool so a target whose
// resolved path lies inside a [protectedDirs] directory is refused with a
// model-facing message instead of executed. Like the read/mutation guards, the
// refusal is a normal result (not an error), so the model adapts rather
// than the run aborting. Resolution uses the canonical physical path, so a
// traversal or symlink that lands in a protected directory is caught too. Apply it as
// the OUTERMOST wrap so the check gates before any staleness/diagnostics work.
//
// In an ISOLATED session it additionally confines writes to the workspace copy:
// the fs executor is not an OS jail (an absolute path, "../", or "~" escapes its
// root), so this guard is the boundary that keeps an isolated run from modifying
// the real project tree. Non-isolated Runs keep the existing behavior (absolute
// paths anywhere are allowed — that is the point of the fs tools).
func withPathGuard(inner toolcontract.Tool, workdir string) toolcontract.Tool {
	return decorate(inner, func(ctx context.Context, arguments string) (string, error) {
		isolated := executionctx.Isolated(ctx)
		paths, err := mutationPaths(inner, arguments)
		if err != nil {
			return "", fmt.Errorf("inspect mutation paths: %w", err)
		}
		for _, path := range paths {
			if refusal, ok := guardMutationPath(workdir, path, isolated); !ok {
				return refusal, nil
			}
		}
		return inner.Call(ctx, arguments)
	})
}

// guardMutationPath decides whether a mutation of path (relative to workdir) is
// allowed, returning ok=false plus a model-facing refusal otherwise. A path is
// refused when it cannot be resolved, lands inside a [protectedDirs] directory,
// or — for an isolated session — escapes the workspace copy. Pure (no ctx) so
// the boundary decision is directly testable.
func guardMutationPath(workdir, path string, isolated bool) (refusal string, ok bool) {
	resolved, err := pathidentity.Resolve(workdir, path)
	if err != nil {
		return fmt.Sprintf("Refused: %q could not be resolved safely (%v).", path, err), false
	}
	if dir := protectedDirHit(resolved); dir != "" {
		return fmt.Sprintf("Refused: %q is inside the protected %q directory, which is read-only to the agent. Use the shell/git tooling if you need to change version-control state.", path, dir), false
	}
	if isolated {
		inside, err := withinWorkspace(workdir, resolved)
		if err != nil {
			return fmt.Sprintf("Refused: %q could not be checked against the sandbox boundary (%v).", path, err), false
		}
		if !inside {
			return fmt.Sprintf("Refused: %q is outside this isolated session's sandbox workspace. An isolated run may only modify files inside its workspace copy.", path), false
		}
	}
	return "", true
}

// withinWorkspace reports whether the resolved path is workdir or below it, with
// workdir resolved to its physical identity first so a symlinked workspace root
// (macOS temp dirs live under /var → /private/var) compares correctly.
func withinWorkspace(workdir, resolved string) (bool, error) {
	root, err := pathidentity.Resolve(workdir, ".")
	if err != nil {
		return false, err
	}
	return pathidentity.Contains(root, resolved)
}

// protectedDirHit returns the [protectedDirs] name when abs lies inside one
// (any path component matches), else "". Walks up via filepath.Base/Dir so
// it is separator-agnostic and matches a protected dir at any depth (a
// nested repo's .git included).
func protectedDirHit(abs string) string {
	for p := abs; ; {
		if base := filepath.Base(p); slices.Contains(protectedDirs, base) {
			return base
		}
		parent := filepath.Dir(p)
		if parent == p {
			return ""
		}
		p = parent
	}
}
