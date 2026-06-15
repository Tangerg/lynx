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
	"sync"
)

// ErrUnavailable means there is no snapshot to restore for the requested run
// (no shadow repo, or no tag at that boundary). It maps to the wire
// checkpoint_unavailable.
var ErrUnavailable = errors.New("checkpoint: no snapshot for run")

// Store manages the shadow repos for every session. Safe for concurrent use:
// each session's shadow-repo operations are serialized by a per-session mutex
// (locks), because a run's snapshot now runs asynchronously off the run-finish
// path — so the next run can start before the previous snapshot finishes, and
// two concurrent git commands on one repo would otherwise race the index lock.
type Store struct {
	root  string   // base dir holding one shadow GIT_DIR per session
	locks sync.Map // sessionID → *sync.Mutex, serializing that repo's git ops
}

// NewStore roots the shadow repos at dir (e.g. <LYRA_HOME>/checkpoints).
func NewStore(dir string) *Store { return &Store{root: dir} }

// lockFor returns the per-session mutex serializing one shadow repo's git ops.
func (s *Store) lockFor(sessionID string) *sync.Mutex {
	mu, _ := s.locks.LoadOrStore(sessionID, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// commonExcludes keep a checkpoint from ballooning into dependency / build
// output in a repo that lacks its own .gitignore. A repo WITH a .gitignore is
// already honored by git; this is only a backstop so a no-ignore project
// doesn't snapshot node_modules on every run.
const commonExcludes = "node_modules/\n.venv/\nvenv/\n__pycache__/\ndist/\nbuild/\ntarget/\n.next/\n.DS_Store\n"

// maxCheckpointFileSize caps a single file the checkpoint will stage (opencode's
// 2 MiB guard). A large unignored binary — a dataset, a built artifact a project
// forgot to .gitignore — would otherwise bloat every snapshot and the shadow
// repo. Oversize files are left out, so a restore won't revert them: an
// acceptable trade-off against unbounded growth.
const maxCheckpointFileSize = 2 << 20

// Snapshot anchors the current state of cwd at the runID boundary: it stages
// the run's changed files (honoring .gitignore + a backstop exclude list +
// the size cap) and tags the commit by run id. An empty commit is allowed so a
// no-change run still anchors a restorable point. Idempotent per run (the tag
// is moved).
func (s *Store) Snapshot(ctx context.Context, sessionID, cwd, runID string) error {
	mu := s.lockFor(sessionID)
	mu.Lock()
	defer mu.Unlock()
	gitDir, err := s.ensureRepo(ctx, sessionID, cwd)
	if err != nil {
		return err
	}
	if err := s.stageChanges(ctx, gitDir, cwd); err != nil {
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

// stageChanges stages the work tree's changes into the shadow index, skipping
// files over [maxCheckpointFileSize]. Only the run's changed / new / removed
// paths are considered — `git ls-files` honors .gitignore + info/exclude — so
// the cost scales with the change set, never the whole tree (the reason we no
// longer `git add -A`, which on a huge / unbounded dir staged everything). The
// resulting commit still reflects the full tree: the index carries unchanged
// files forward from the prior commit.
func (s *Store) stageChanges(ctx context.Context, gitDir, cwd string) error {
	out, err := s.git(ctx, gitDir, cwd, "ls-files", "-z",
		"--modified", "--others", "--deleted", "--exclude-standard")
	if err != nil {
		return err
	}
	var paths []string
	for p := range strings.SplitSeq(out, "\x00") {
		if p == "" {
			continue
		}
		// A deletion (Stat fails) is staged so the commit records the removal;
		// a present file is staged only when it's under the size cap.
		if info, err := os.Stat(filepath.Join(cwd, p)); err == nil {
			if info.IsDir() || info.Size() > maxCheckpointFileSize {
				continue
			}
		}
		paths = append(paths, p)
	}
	// Stage in chunks so a large change set can't overflow the arg limit.
	const chunk = 256
	for i := 0; i < len(paths); i += chunk {
		args := append([]string{"add", "--"}, paths[i:min(i+chunk, len(paths))]...)
		if _, err := s.git(ctx, gitDir, cwd, args...); err != nil {
			return err
		}
	}
	return nil
}

// Restore resets cwd's work tree to the runID snapshot: tracked files revert,
// and files created-and-staged since (small enough to be tracked) are removed
// by the reset. Ignored files AND oversize files (never staged — see
// [maxCheckpointFileSize]) are left untouched: they're untracked, so the reset
// doesn't reach them, and we deliberately do NOT `git clean` (that would delete
// the user's large/ignored files the checkpoint never owned). The current state
// is auto-committed first so the restore is itself reversible (unrevert).
// Returns ErrUnavailable when runID has no snapshot.
func (s *Store) Restore(ctx context.Context, sessionID, cwd, runID string) error {
	mu := s.lockFor(sessionID)
	mu.Lock()
	defer mu.Unlock()
	gitDir := s.gitDir(sessionID)
	if !repoExists(gitDir) {
		return ErrUnavailable
	}
	if _, err := s.git(ctx, gitDir, cwd, "rev-parse", "-q", "--verify", "refs/tags/"+tagFor(runID)); err != nil {
		return ErrUnavailable
	}
	// Reversibility: capture the pre-restore state as a commit before resetting.
	if err := s.stageChanges(ctx, gitDir, cwd); err != nil {
		return err
	}
	_, _ = s.git(ctx, gitDir, cwd, "commit", "-q", "--allow-empty", "-m", "pre-restore")
	// reset --hard reverts tracked files + drops tracked files created since the
	// target. No `git clean`: untracked files (ignored, or oversize and so never
	// staged) are not the checkpoint's to delete.
	if _, err := s.git(ctx, gitDir, cwd, "reset", "-q", "--hard", tagFor(runID)); err != nil {
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
