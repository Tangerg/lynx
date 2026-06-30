package toolset

import (
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
