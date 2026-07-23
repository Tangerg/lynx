// Package workspacepath resolves filesystem paths at the adapter boundary.
// Canonical working-directory identity is an external fact (absolute path,
// symlink target, existence), not a domain rule.
package workspacepath

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
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

// ResolveInRoot lexically confines a client path to root and returns its
// root-relative form. The workspace application owns the input-policy errors;
// this adapter owns filesystem path spelling and cleaning.
func (Resolver) ResolveInRoot(root, path string) (string, error) {
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
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, rel))
	if err != nil {
		return rel, nil
	}
	if !pathInside(Canonical(root), Canonical(resolved)) {
		return "", workspaceapp.ErrPathOutsideRoot
	}
	return rel, nil
}

// Inspect derives the live workspace projection for an already-admitted cwd.
// A path that disappeared (or was replaced by a non-directory) is a normal
// domain state, not an error. Other filesystem failures remain explicit.
func (Resolver) Inspect(path string) (session.WorkspaceIdentity, error) {
	if path == "" {
		return session.WorkspaceIdentity{Missing: true}, nil
	}
	cwd := Canonical(path)
	identity := session.WorkspaceIdentity{Cwd: cwd, ProjectRoot: cwd}
	info, err := os.Stat(cwd)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			identity.Missing = true
			return identity, nil
		}
		return session.WorkspaceIdentity{}, err
	}
	if !info.IsDir() {
		identity.Missing = true
		return identity, nil
	}

	root, err := nearestProjectRoot(cwd)
	if err != nil {
		return session.WorkspaceIdentity{}, err
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

func pathInside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
