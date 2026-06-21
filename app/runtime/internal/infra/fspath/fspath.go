// Package fspath holds filesystem-path helpers shared across the adapters.
package fspath

import "path/filepath"

// Canonical normalizes a path to a stable identity: absolute, with symlinks
// resolved. Different spellings of one location (trailing slash, relative path,
// a symlinked ancestor) collapse to the same string — so a path used as a map
// key or a lock key (a working-tree lock, a per-cwd index) identifies the
// LOCATION, not the spelling, and two callers naming one directory differently
// can't take two keys for it. Empty stays empty (the canonical form of "no
// path" is "no path"). Falls back to the cleaned absolute path when the target
// can't be resolved (e.g. it doesn't exist yet), and to a cleaned input as a
// last resort.
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
