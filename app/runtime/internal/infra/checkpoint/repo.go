package checkpoint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// commonExcludes keep a checkpoint from ballooning into dependency / build
// output in a repo that lacks its own .gitignore. A repo WITH a .gitignore is
// already honored by git; this is only a backstop so a no-ignore project
// doesn't snapshot node_modules on every run.
const commonExcludes = "node_modules/\n.venv/\nvenv/\n__pycache__/\ndist/\nbuild/\ntarget/\n.next/\n.DS_Store\n"

// ensureRepo lazily initializes the session's shadow repo (idempotent).
func (s *Store) ensureRepo(ctx context.Context, sessionID, cwd string) (string, error) {
	gitDir := s.gitDir(sessionID)
	if repoExists(gitDir) {
		return gitDir, nil
	}
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		return "", fmt.Errorf("checkpoint: mkdir: %w", err)
	}
	if _, err := s.git(ctx, gitDir, cwd, "init", "-q"); err != nil {
		return "", err
	}
	s.seedFrom(ctx, gitDir, cwd)
	if err := os.WriteFile(filepath.Join(gitDir, "info", "exclude"), []byte(commonExcludes), 0o644); err != nil {
		return "", fmt.Errorf("checkpoint: write excludes: %w", err)
	}
	return gitDir, nil
}

// seedFrom wires a freshly-initialized shadow repo to reuse the real repo's
// object store and index, so the first snapshot doesn't re-store the whole tree.
// Sharing objects/info/alternates lets `git add` resolve every unchanged blob
// through the real .git instead of writing a copy; seeding the index reuses the
// existing hashes instead of re-hashing every file — the cost that becomes
// significant on large checkouts.
//
// Best-effort: if cwd isn't a git repo, or anything is missing, the shadow just
// starts empty and the first `git add` does the full work — correct, only
// slower. The one trade-off: a shared object pruned from the real repo (only
// possible once it's unreachable there) would leave a snapshot that referenced
// it unrestorable — acceptable for a best-effort file checkpoint.
func (s *Store) seedFrom(ctx context.Context, gitDir, cwd string) {
	common, err := gitIn(ctx, cwd, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || common == "" {
		return // not a git repo we can seed from
	}
	srcObjects := filepath.Join(common, "objects")
	if _, err := os.Stat(srcObjects); err != nil {
		return
	}
	// Share the real object DB plus any store it already borrows, keeping only
	// the ones that still exist so the chain resolves.
	alternates := []string{srcObjects}
	if data, err := os.ReadFile(filepath.Join(srcObjects, "info", "alternates")); err == nil {
		for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
			if p := strings.TrimSpace(line); p != "" {
				if _, err := os.Stat(p); err == nil {
					alternates = append(alternates, p)
				}
			}
		}
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "objects", "info"), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(gitDir, "objects", "info", "alternates"),
		[]byte(strings.Join(alternates, "\n")+"\n"), 0o644)
	if src := filepath.Join(common, "index"); fileExists(src) {
		_ = copyFile(src, filepath.Join(gitDir, "index"))
	}
}

func repoExists(gitDir string) bool {
	_, err := os.Stat(filepath.Join(gitDir, "HEAD"))
	return err == nil
}

// tagFor maps a run id to its snapshot ref name, sanitizing any character git
// disallows in a ref so an unusual run id can't break tagging.
func tagFor(runID string) string {
	var b strings.Builder
	b.WriteString("chk/")
	for _, r := range runID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}
