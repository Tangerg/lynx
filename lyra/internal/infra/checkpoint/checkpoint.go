// Package checkpoint snapshots a session's working tree at run boundaries via
// a per-session SHADOW git repository, so a rollback or fork can restore files
// (not just chat history) to a chosen run.
//
// The shadow repo's GIT_DIR lives under the lyra home, with the session's cwd
// as its work tree — the user's own .git is never touched (git addresses the
// two independently, the classic dotfiles-repo pattern). Each run boundary is
// anchored by a lightweight tag named for the run id, so a restore is a reset
// to that tag. The only OS dependency is the git binary, which lyra already
// requires for workspace diffs — so this is platform-agnostic.
package checkpoint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrUnavailable means there is no snapshot to restore for the requested run
// (no shadow repo, or no tag at that boundary). It maps to the wire
// checkpoint_unavailable.
var ErrUnavailable = errors.New("checkpoint: no snapshot for run")

// Store manages the shadow repos for every session. Safe for concurrent use
// across sessions; git serializes operations within one repo via its index
// lock, and lyra drives at most one run per session at a time.
type Store struct {
	root string // base dir holding one shadow GIT_DIR per session
}

// NewStore roots the shadow repos at dir (e.g. <LYRA_HOME>/checkpoints).
func NewStore(dir string) *Store { return &Store{root: dir} }

// commonExcludes keep a checkpoint from ballooning into dependency / build
// output in a repo that lacks its own .gitignore. A repo WITH a .gitignore is
// already honored by git; this is only a backstop so a no-ignore project
// doesn't snapshot node_modules on every run.
const commonExcludes = "node_modules/\n.venv/\nvenv/\n__pycache__/\ndist/\nbuild/\ntarget/\n.next/\n.DS_Store\n"

// Snapshot anchors the current state of cwd at the runID boundary: it stages
// the whole work tree (honoring .gitignore + a backstop exclude list) and tags
// the commit by run id. An empty commit is allowed so a no-change run still
// anchors a restorable point. Idempotent per run (the tag is moved).
func (s *Store) Snapshot(ctx context.Context, sessionID, cwd, runID string) error {
	gitDir, err := s.ensureRepo(ctx, sessionID, cwd)
	if err != nil {
		return err
	}
	if _, err := s.git(ctx, gitDir, cwd, "add", "-A"); err != nil {
		return err
	}
	if _, err := s.git(ctx, gitDir, cwd, "commit", "-q", "--allow-empty", "-m", "run "+runID); err != nil {
		return err
	}
	if _, err := s.git(ctx, gitDir, cwd, "tag", "-f", tagFor(runID)); err != nil {
		return err
	}
	return nil
}

// Restore resets cwd's work tree to the runID snapshot: tracked files are
// reverted and files created since are removed (ignored files are left alone).
// The current state is auto-committed first so the restore is itself
// reversible (unrevert). Returns ErrUnavailable when runID has no snapshot.
func (s *Store) Restore(ctx context.Context, sessionID, cwd, runID string) error {
	gitDir := s.gitDir(sessionID)
	if !repoExists(gitDir) {
		return ErrUnavailable
	}
	if _, err := s.git(ctx, gitDir, cwd, "rev-parse", "-q", "--verify", "refs/tags/"+tagFor(runID)); err != nil {
		return ErrUnavailable
	}
	// Reversibility: capture the pre-restore state as a commit before resetting.
	if _, err := s.git(ctx, gitDir, cwd, "add", "-A"); err != nil {
		return err
	}
	_, _ = s.git(ctx, gitDir, cwd, "commit", "-q", "--allow-empty", "-m", "pre-restore")
	if _, err := s.git(ctx, gitDir, cwd, "reset", "-q", "--hard", tagFor(runID)); err != nil {
		return err
	}
	if _, err := s.git(ctx, gitDir, cwd, "clean", "-fdq"); err != nil {
		return err
	}
	return nil
}

// DropSession removes a session's shadow repo (on session delete).
func (s *Store) DropSession(sessionID string) error {
	return os.RemoveAll(s.gitDir(sessionID))
}

func (s *Store) gitDir(sessionID string) string { return filepath.Join(s.root, sessionID) }

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
	if err := os.WriteFile(filepath.Join(gitDir, "info", "exclude"), []byte(commonExcludes), 0o644); err != nil {
		return "", fmt.Errorf("checkpoint: write excludes: %w", err)
	}
	return gitDir, nil
}

// git runs one git command against the shadow GIT_DIR with cwd as the work
// tree (workTree may be empty for repo-only operations like rev-parse). A
// fixed identity + disabled signing keep commits independent of the user's
// global git config.
func (s *Store) git(ctx context.Context, gitDir, workTree string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	env := append(os.Environ(),
		"GIT_DIR="+gitDir,
		"GIT_AUTHOR_NAME=lyra", "GIT_AUTHOR_EMAIL=lyra@localhost",
		"GIT_COMMITTER_NAME=lyra", "GIT_COMMITTER_EMAIL=lyra@localhost",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	)
	if workTree != "" {
		env = append(env, "GIT_WORK_TREE="+workTree)
	}
	cmd.Env = env
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("checkpoint: git %s: %w: %s", args[0], err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
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
