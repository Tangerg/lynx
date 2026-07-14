package toolset

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/Tangerg/lynx/tools"
)

// protectedDirs are directory names the agent must never write into, even
// when they sit inside the workspace. Writing under .git (a hook, the
// config) is a remote-code-execution / repo-hijack vector, so the VCS
// metadata directory is carved read-only regardless of approval mode — the
// standard invariant enforced on writable roots. A model that needs
// to change version-control state uses the shell/git tooling, not a raw
// file write. Kept as a list so other state dirs can join if a need arises.
var protectedDirs = []string{".git"}

// withPathGuard wraps a file-mutating tool (write / edit) so a write whose
// resolved path lies inside a [protectedDirs] directory is refused with a
// model-facing message instead of executed. Like the read/edit guards the
// refusal is a normal result (not an error), so the model adapts rather
// than the run aborting. Resolution runs through [resolveAbs], so a "../"
// traversal that lands in a protected directory is caught too. Apply it as
// the OUTERMOST wrap so the check gates before any staleness/diagnostics work.
func withPathGuard(inner tools.Tool, workdir string) tools.Tool {
	return wrapTool(inner, func(ctx context.Context, arguments string) (string, error) {
		paths := mutatedPaths(inner, arguments)
		for _, path := range paths {
			resolved, err := resolvePhysicalAbs(workdir, path)
			if err != nil {
				return fmt.Sprintf("Refused: %q could not be resolved safely (%v).", path, err), nil
			}
			if dir := protectedDirHit(resolved); dir != "" {
				return fmt.Sprintf("Refused: %q is inside the protected %q directory, which is read-only to the agent. Use the shell/git tooling if you need to change version-control state.", path, dir), nil
			}
		}
		return inner.Call(ctx, arguments)
	})
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
