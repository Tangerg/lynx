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
//
// To avoid re-storing a project that git already has, a fresh shadow repo SEEDS
// itself from the real repo at cwd (see [Store.seedFrom]): it shares the real
// .git object store via objects/info/alternates and copies its index, so the
// baseline snapshot references existing blobs and skips re-hashing unchanged
// files — instead of duplicating the whole working tree on the first run.
package checkpoint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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

// maxCheckpointFileSize caps a single file the checkpoint will stage (2 MiB
// guard). A large unignored binary — a dataset, a built artifact a project
// forgot to .gitignore — would otherwise bloat every snapshot and the shadow
// repo. Oversize files are left out, so a restore won't revert them: an
// acceptable trade-off against unbounded growth.
const maxCheckpointFileSize = 2 << 20

// Snapshot anchors the current state of cwd at the runID boundary: it stages
// the run's changed files (honoring .gitignore + a backstop exclude list +
// the size cap) and tags the boundary by run id. A run that changed nothing
// re-tags the existing HEAD instead of minting an empty commit, so a no-change
// turn costs one ref and zero objects. Idempotent per run (the tag is moved).
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
	// Commit only when the staged tree actually differs from HEAD (or there is
	// no HEAD yet — the baseline). A no-change run skips the commit and just
	// re-points its tag at HEAD, so it adds a ref but no empty commit object.
	if s.shouldCommit(ctx, gitDir, cwd) {
		if _, err := s.git(ctx, gitDir, cwd, "commit", "-q", "--allow-empty", "-m", "run "+runID); err != nil {
			return err
		}
	}
	if _, err := s.git(ctx, gitDir, cwd, "tag", "-f", tagFor(runID)); err != nil {
		return err
	}
	return nil
}

// shouldCommit reports whether the staged index warrants a new commit. The
// first snapshot (no HEAD yet) always commits the baseline; afterwards a commit
// is made only when the staged tree differs from HEAD — so a no-change run
// re-tags the existing HEAD rather than minting an empty commit.
func (s *Store) shouldCommit(ctx context.Context, gitDir, cwd string) bool {
	if _, err := s.git(ctx, gitDir, cwd, "rev-parse", "-q", "--verify", "HEAD"); err != nil {
		return true // no HEAD → the baseline commit
	}
	// `diff --cached --quiet HEAD` exits non-zero exactly when the staged tree
	// differs from HEAD; any error (a real diff, or the rare git failure) is
	// resolved toward committing, so a boundary is never silently lost.
	_, err := s.git(ctx, gitDir, cwd, "diff", "--cached", "--quiet", "HEAD")
	return err != nil
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
// the user's large/ignored files the checkpoint never owned). The pre-restore
// state is captured as a commit first (when it differs from HEAD) so the
// restore is itself reversible (unrevert). Returns ErrUnavailable when runID
// has no snapshot.
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
	// Reversibility: capture the pre-restore state as a commit before resetting,
	// but only when there's something to capture (no empty commit otherwise).
	if err := s.stageChanges(ctx, gitDir, cwd); err != nil {
		return err
	}
	if s.shouldCommit(ctx, gitDir, cwd) {
		// The pre-restore commit is what makes the restore reversible (unrevert).
		// If it fails, do NOT proceed to the destructive reset below — that would
		// discard the working-tree state with no recovery point, turning a
		// "reversible" restore irreversible. Fail instead.
		if _, err := s.git(ctx, gitDir, cwd, "commit", "-q", "-m", "pre-restore"); err != nil {
			return err
		}
	}
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
	s.seedFrom(ctx, gitDir, cwd)
	if err := os.WriteFile(filepath.Join(gitDir, "info", "exclude"), []byte(commonExcludes), 0o644); err != nil {
		return "", fmt.Errorf("checkpoint: write excludes: %w", err)
	}
	return gitDir, nil
}

// seedFrom wires a freshly-initialized shadow repo to reuse the real repo's
// object store and index, so the first snapshot doesn't re-store the whole tree.
// Sharing objects/info/alternates lets `git add` resolve
// every unchanged blob through the real .git instead of writing a copy; seeding
// the index reuses the existing hashes instead of re-hashing every file — the
// cost that becomes significant on large checkouts.
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

// gitIn runs a git query inside the real repo at cwd (no shadow GIT_DIR), used
// to discover what a new shadow repo can seed from. Returns trimmed stdout.
func gitIn(ctx context.Context, cwd string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
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
