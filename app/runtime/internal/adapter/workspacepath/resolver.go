// Package workspacepath resolves filesystem paths at the adapter boundary.
// Canonical working-directory identity is an external fact (absolute path,
// symlink target, existence), not a domain rule.
package workspacepath

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrNotDirectory reports that a path exists but is not a directory.
var ErrNotDirectory = errors.New("workspacepath: not a directory")

// Canonical returns the stable identity used for working-tree locks,
// checkpoints, and per-cwd indexes. Missing paths are still normalized to an
// absolute spelling; callers that require existence use Resolver.
func Canonical(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

// Resolver implements the application session coordinator's cwd-resolution
// port.
type Resolver struct{}

// ResolveExistingDir verifies path exists as a directory and returns its
// canonical identity.
func (Resolver) ResolveExistingDir(path string) (string, error) {
	return ResolveExistingDir(path)
}

// ResolveExistingDir is the functional form used by outer adapters that do not
// need to hold a resolver port value.
func ResolveExistingDir(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", ErrNotDirectory
	}
	return Canonical(path), nil
}
