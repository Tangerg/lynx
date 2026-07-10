package toolset

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolveAbs resolves a tool's file_path argument to an absolute path for the
// guards' bookkeeping (read-before-edit key, staleness stat, .git protection).
// It must agree with how the fs executor resolves the same argument — including
// expanding a leading ~ to the home dir — or the guards would track/stat a
// different path than the one actually read or written.
func resolveAbs(workdir, path string) string {
	path = expandHome(path)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(workdir, path))
}

// resolvePhysicalAbs gives aliases of the same file one identity. It resolves
// every existing symlink component while allowing a not-yet-created suffix, a
// shape filepath.EvalSymlinks alone cannot handle. This identity backs path
// locks, stale-read stamps, and protected-directory checks.
func resolvePhysicalAbs(workdir, path string) (string, error) {
	return resolvePhysicalPath(resolveAbs(workdir, path), 0)
}

func canonicalAbs(workdir, path string) string {
	abs := resolveAbs(workdir, path)
	resolved, err := resolvePhysicalPath(abs, 0)
	if err != nil {
		return abs
	}
	return resolved
}

const maxSymlinkDepth = 40

func resolvePhysicalPath(abs string, depth int) (string, error) {
	if depth > maxSymlinkDepth {
		return "", errors.New("too many symbolic links")
	}
	current := filepath.Clean(abs)
	var suffix []string
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(append([]string{resolved}, suffix...)...), nil
		}

		info, lstatErr := os.Lstat(current)
		if lstatErr == nil {
			if info.Mode()&os.ModeSymlink == 0 {
				return "", fmt.Errorf("cannot resolve path %q", abs)
			}
			target, err := os.Readlink(current)
			if err != nil {
				return "", fmt.Errorf("read symlink %q: %w", current, err)
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(current), target)
			}
			resolved, err := resolvePhysicalPath(target, depth+1)
			if err != nil {
				return "", err
			}
			return filepath.Join(append([]string{resolved}, suffix...)...), nil
		}
		if !os.IsNotExist(lstatErr) {
			return "", fmt.Errorf("inspect path %q: %w", current, lstatErr)
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("cannot resolve path %q", abs)
		}
		suffix = append([]string{filepath.Base(current)}, suffix...)
		current = parent
	}
}

// expandHome expands a leading ~ (the shell convention an LLM routinely emits)
// to the current user's home dir, matching tools/fs's executor. "~"/"~/" →
// home, "~/x" → home/x; any other form is unchanged. Best-effort on a home-dir
// lookup failure. Duplicated from tools/fs by design — 8 lines across a module
// boundary beats a shared dep (root CLAUDE.md: DRY < 3× ⇒ prefer repeat).
func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[len("~/"):])
}

func isExistingFile(abs string) bool {
	info, err := os.Stat(abs)
	return err == nil && !info.IsDir()
}
