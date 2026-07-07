package checkpoint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

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
	mu := s.lockFor(cwd)
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
