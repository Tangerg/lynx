// Package workspace contains focused project-scoped application use cases for
// workspace identity, browsing, knowledge, skills, hooks, and Git observation.
package workspace

import (
	"errors"
	"fmt"
)

// Workspace input failures are stable application errors.
var (
	ErrCWDUnavailable     = errors.New("workspace: cwd unavailable")
	ErrPathRequired       = errors.New("workspace: path required")
	ErrPathOutsideRoot    = errors.New("workspace: path outside root")
	ErrInvalidFileRange   = errors.New("workspace: invalid file range")
	ErrFileReadTooLarge   = errors.New("workspace: file read exceeds its resource limit")
	ErrUnsupportedText    = errors.New("workspace: file is not UTF-8 text")
	ErrGrepQueryMissing   = errors.New("workspace: grep query required")
	ErrInvalidGrepQuery   = errors.New("workspace: invalid grep query")
	ErrGrepResultTooLarge = errors.New("workspace: file search exceeds its resource limit")
)

// Paths resolves the externally-observed filesystem identity used by workspace
// use cases. Implementations own path canonicalization and symlink inspection;
// this package owns when each operation is required.
type Paths interface {
	ResolveExistingDir(path string) (string, error)
	ResolveInRoot(root, path string) (string, error)
	ResolveExistingInRoot(root, path string) (string, error)
}

// Scope resolves the workspace identity shared by independent use cases.
type Scope struct {
	defaultWorkspacePath string
	userHome             string
	paths                Paths
}

// NewScope constructs the shared workspace root scope.
func NewScope(defaultWorkspacePath, userHome string, paths Paths) *Scope {
	return &Scope{defaultWorkspacePath: defaultWorkspacePath, userHome: userHome, paths: paths}
}

func (s *Scope) root(cwd string) (string, error) {
	root := cwd
	if root == "" {
		root = s.defaultWorkspacePath
	}
	if s.paths == nil {
		return "", fmt.Errorf("%w: path resolver is not configured", ErrCWDUnavailable)
	}
	resolved, err := s.paths.ResolveExistingDir(root)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrCWDUnavailable, root, err)
	}
	return resolved, nil
}

// ResolveRoot returns the effective, existing working directory for a workspace
// request. Empty cwd selects the host-provided default workspace.
func (s *Scope) ResolveRoot(cwd string) (string, error) {
	return s.root(cwd)
}
