// Package pathidentity gives filesystem aliases one stable physical identity
// for filesystem-facing adapters.
package pathidentity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxSymlinkDepth = 40

// absolute anchors a relative path under an explicit absolute root. Host cwd
// and User Home discovery belong to process composition, never path identity.
func absolute(root, path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	if root == "" {
		return "", errors.New("resolve filesystem path: root is required for a relative path")
	}
	if !filepath.IsAbs(root) {
		return "", errors.New("resolve filesystem path: root must be absolute")
	}
	return filepath.Clean(filepath.Join(root, path)), nil
}

// Resolve resolves every existing symlink component while preserving a
// not-yet-created suffix. This is the identity used by path locks and security
// decisions, so aliases cannot disagree about the resource they target.
func Resolve(root, path string) (string, error) {
	abs, err := absolute(root, path)
	if err != nil {
		return "", err
	}
	return resolve(abs, 0)
}

// Canonical returns the physical identity when it can be established and the
// absolute lexical identity otherwise. Use Resolve instead when failure must
// be handled conservatively at a trust boundary.
func Canonical(root, path string) (string, error) {
	abs, err := absolute(root, path)
	if err != nil {
		return "", err
	}
	resolved, err := resolve(abs, 0)
	if err != nil {
		return abs, nil
	}
	return resolved, nil
}

// Contains reports whether target is root itself or lies below it. Callers at
// a filesystem trust boundary must pass values produced by Resolve so symlink
// aliases have already been eliminated.
func Contains(root, target string) (bool, error) {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false, fmt.Errorf("compare filesystem paths: %w", err)
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
}

func resolve(abs string, depth int) (string, error) {
	if depth > maxSymlinkDepth {
		return "", errors.New("too many symbolic links")
	}
	current := filepath.Clean(abs)
	var suffix []string
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(append([]string{resolved}, suffix...)...), nil
		}

		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink == 0 {
				return "", fmt.Errorf("resolve filesystem path %q: existing prefix is not traversable", abs)
			}
			target, readErr := os.Readlink(current)
			if readErr != nil {
				return "", fmt.Errorf("read symbolic link %q: %w", current, readErr)
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(current), target)
			}
			resolved, resolveErr := resolve(target, depth+1)
			if resolveErr != nil {
				return "", resolveErr
			}
			return filepath.Join(append([]string{resolved}, suffix...)...), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect filesystem path %q: %w", current, err)
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("resolve filesystem path %q: no existing ancestor", abs)
		}
		suffix = append([]string{filepath.Base(current)}, suffix...)
		current = parent
	}
}
