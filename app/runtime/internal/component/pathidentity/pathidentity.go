// Package pathidentity gives filesystem aliases one stable physical identity.
package pathidentity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxSymlinkDepth = 40

// Absolute anchors path under root, matching the local filesystem executor's
// treatment of relative and home-relative paths.
func Absolute(root, path string) string {
	path = expandHome(path)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if root != "" {
		return filepath.Clean(filepath.Join(root, path))
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

// Resolve resolves every existing symlink component while preserving a
// not-yet-created suffix. This is the identity used by path locks and security
// decisions, so aliases cannot disagree about the resource they target.
func Resolve(root, path string) (string, error) {
	return resolve(Absolute(root, path), 0)
}

// Canonical returns the physical identity when it can be established and the
// absolute lexical identity otherwise. Use Resolve instead when failure must
// be handled conservatively at a trust boundary.
func Canonical(root, path string) string {
	abs := Absolute(root, path)
	resolved, err := resolve(abs, 0)
	if err != nil {
		return abs
	}
	return resolved
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

func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[len("~/"):])
}
