// Package workspacepath resolves filesystem paths at the adapter boundary.
// Canonical working-directory identity is an external fact (absolute path,
// symlink target, existence), not a domain rule.
package workspacepath

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	workspaceapp "github.com/Tangerg/scope/app/runtime/internal/application/workspace"
	"github.com/Tangerg/scope/app/runtime/internal/infra/pathidentity"
)

// ErrNotDirectory reports that a path exists but is not a directory.
var ErrNotDirectory = errors.New("workspacepath: not a directory")

// ErrAbsolutePathRequired reports an attempt to let an adapter rediscover the
// process cwd. Workspace roots are resolved once at process/session boundaries.
var ErrAbsolutePathRequired = errors.New("workspacepath: absolute path required")

// Canonical returns the stable identity used for working-tree locks,
// checkpoints, and per-cwd indexes. Missing paths are still normalized to an
// absolute spelling; callers that require existence use Resolver. Relative
// values are rejected because resolving them would depend on the process cwd.
func Canonical(path string) (string, error) {
	canonical, err := pathidentity.Canonical("", path)
	if err != nil {
		return "", ErrAbsolutePathRequired
	}
	return canonical, nil
}

// Resolver implements the application session coordinator's cwd-resolution
// port.
type Resolver struct{}

// ResolveExistingDir verifies path exists as a directory and returns its
// canonical identity.
func (Resolver) ResolveExistingDir(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", ErrAbsolutePathRequired
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", ErrNotDirectory
	}
	return Canonical(path)
}

// ResolveInRoot lexically confines a client path to root and returns its
// root-relative form. The workspace application owns the input-policy errors;
// this adapter owns filesystem path spelling and cleaning.
func (Resolver) ResolveInRoot(root, path string) (string, error) {
	if !filepath.IsAbs(root) {
		return "", ErrAbsolutePathRequired
	}
	if path == "" {
		return "", workspaceapp.ErrPathRequired
	}
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, path)
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", workspaceapp.ErrPathOutsideRoot
	}
	return rel, nil
}

// ResolveExistingInRoot also resolves existing symlinks before returning a
// root-relative path, preventing a file read from escaping through an in-root
// symlink. Missing targets are left for the consuming file adapter to report.
func (r Resolver) ResolveExistingInRoot(root, path string) (string, error) {
	rel, err := r.ResolveInRoot(root, path)
	if err != nil {
		return "", err
	}
	physicalRoot, err := pathidentity.Resolve("", root)
	if err != nil {
		return "", err
	}
	physicalTarget, err := pathidentity.Resolve(physicalRoot, rel)
	if err != nil {
		return "", err
	}
	inside, err := pathidentity.Contains(physicalRoot, physicalTarget)
	if err != nil {
		return "", err
	}
	if !inside {
		return "", workspaceapp.ErrPathOutsideRoot
	}
	return rel, nil
}

// Inspect derives the live workspace projection for an already-admitted cwd.
// A path that disappeared (or was replaced by a non-directory) is a normal
// workspace projection, not an error. Other filesystem failures remain explicit.
func (Resolver) Inspect(path string) (workspaceapp.Resolved, error) {
	if path == "" {
		return workspaceapp.Resolved{Missing: true}, nil
	}
	cwd, err := Canonical(path)
	if err != nil {
		return workspaceapp.Resolved{}, err
	}
	identity := workspaceapp.Resolved{Path: cwd, ProjectRoot: cwd}
	info, err := os.Stat(cwd)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			identity.Missing = true
			return identity, nil
		}
		return workspaceapp.Resolved{}, err
	}
	if !info.IsDir() {
		identity.Missing = true
		return identity, nil
	}

	root, err := nearestProjectRoot(cwd)
	if err != nil {
		return workspaceapp.Resolved{}, err
	}
	identity.ProjectRoot = root
	return identity, nil
}

func nearestProjectRoot(cwd string) (string, error) {
	for dir := cwd; ; dir = filepath.Dir(dir) {
		_, err := os.Stat(filepath.Join(dir, ".git"))
		switch {
		case err == nil:
			return dir, nil
		case !errors.Is(err, os.ErrNotExist):
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd, nil
		}
	}
}
