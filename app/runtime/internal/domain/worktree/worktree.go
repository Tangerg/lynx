package worktree

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrNotDirectory reports that a cwd does not identify an existing directory.
var ErrNotDirectory = errors.New("worktree: not a directory")

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

// ResolveExistingDir verifies cwd exists as a directory and returns its
// canonical identity.
func ResolveExistingDir(cwd string) (string, error) {
	info, err := os.Stat(cwd)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", ErrNotDirectory
	}
	return CanonicalCwd(cwd), nil
}
