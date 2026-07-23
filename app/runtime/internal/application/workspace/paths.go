package workspace

import (
	"errors"
	"fmt"
)

// Workspace input failures are application errors. Delivery maps them to the
// protocol's symbolic errors without leaking protocol values inward.
var (
	ErrCwdUnavailable   = errors.New("workspace: cwd unavailable")
	ErrPathRequired     = errors.New("workspace: path required")
	ErrPathOutsideRoot  = errors.New("workspace: path outside root")
	ErrInvalidFileRange = errors.New("workspace: invalid file range")
	ErrGrepQueryMissing = errors.New("workspace: grep query required")
)

// Paths resolves the externally-observed filesystem identity used by workspace
// use cases. The adapter owns path canonicalization and symlink inspection;
// this package owns when a use case requires each operation.
type Paths interface {
	ResolveExistingDir(path string) (string, error)
	ResolveInRoot(root, path string) (string, error)
	ResolveExistingInRoot(root, path string) (string, error)
}

func (c *Context) root(cwd string) (string, error) {
	root := cwd
	if root == "" {
		root = c.defaultCwd
	}
	if c.paths == nil {
		return "", fmt.Errorf("%w: path resolver is not configured", ErrCwdUnavailable)
	}
	resolved, err := c.paths.ResolveExistingDir(root)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrCwdUnavailable, root, err)
	}
	return resolved, nil
}

// ResolveRoot returns the effective, existing working directory for a workspace
// request. Empty cwd selects the host-provided default working directory.
func (c *Context) ResolveRoot(cwd string) (string, error) {
	return c.root(cwd)
}
