package worktree

import "path/filepath"

// CanonicalCwd normalizes a working-tree directory into the stable identity
// used for locks, live-run lookup, checkpoints, and per-cwd indexes.
func CanonicalCwd(cwd string) string {
	if cwd == "" {
		return ""
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return filepath.Clean(cwd)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}
