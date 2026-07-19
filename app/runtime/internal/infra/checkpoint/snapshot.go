package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	shouldCommit, err := s.shouldCommit(ctx, gitDir, cwd)
	if err != nil {
		return err
	}
	if shouldCommit {
		if _, err := s.git(ctx, gitDir, cwd, "commit", "-q", "--allow-empty", "-m", "run "+runID); err != nil {
			return err
		}
	}
	if err := s.materializeAlternates(ctx, gitDir); err != nil {
		return err
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
func (s *Store) shouldCommit(ctx context.Context, gitDir, cwd string) (bool, error) {
	hasHead, err := s.hasHead(ctx, gitDir)
	if err != nil {
		return false, err
	}
	if !hasHead {
		return true, nil // no HEAD → the baseline commit
	}
	// `diff --cached --quiet HEAD` exits non-zero exactly when the staged tree
	// differs from HEAD. Exit 1 means a diff; every other failure is operational
	// and must be reported rather than disguised as a change.
	_, err = s.git(ctx, gitDir, cwd, "diff", "--cached", "--quiet", "HEAD")
	if err == nil {
		return false, nil
	}
	if gitExitCode(err) == 1 {
		return true, nil
	}
	return false, err
}

func (s *Store) hasHead(ctx context.Context, gitDir string) (bool, error) {
	_, err := s.git(ctx, gitDir, "", "rev-parse", "-q", "--verify", "HEAD")
	if err == nil {
		return true, nil
	}
	if gitExitCode(err) == 1 {
		return false, nil
	}
	return false, err
}

// stageChanges stages the work tree's changes into the shadow index, skipping
// files over [maxCheckpointFileSize]. The baseline inspects all initial index
// entries once to establish ownership; later snapshots consider only changed,
// new, or removed paths. `git ls-files` honors .gitignore + info/exclude, and
// the index carries unchanged files forward without rescanning the whole tree.
func (s *Store) stageChanges(ctx context.Context, gitDir, cwd string) error {
	hasHead, err := s.hasHead(ctx, gitDir)
	if err != nil {
		return err
	}
	args := []string{"ls-files", "-z", "--modified", "--others", "--deleted", "--exclude-standard"}
	if !hasHead {
		// A copied source index may contain unchanged files. Include every cached
		// path at the baseline so the size policy is applied before the first tree
		// is committed; subsequent snapshots only inspect changed paths.
		args = append(args, "--cached")
	}
	out, err := s.git(ctx, gitDir, cwd, args...)
	if err != nil {
		return err
	}
	var stage, untrack []string
	seen := make(map[string]struct{})
	for p := range strings.SplitSeq(out, "\x00") {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		// A missing path is a deletion and must be staged. Other inspection
		// failures are operational errors, not evidence that the file vanished.
		info, err := os.Lstat(filepath.Join(cwd, p))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				stage = append(stage, p)
				continue
			}
			return fmt.Errorf("checkpoint: inspect %q: %w", p, err)
		}
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if info.Size() > maxCheckpointFileSize {
			// A path can cross the cap after an earlier snapshot, or arrive in a
			// copied source index already oversized. Remove it from the shadow index
			// without touching the work tree so the checkpoint stops owning it.
			untrack = append(untrack, p)
			continue
		}
		stage = append(stage, p)
	}
	if err := s.updateIndex(ctx, gitDir, cwd,
		[]string{"rm", "-q", "-f", "--cached", "--ignore-unmatch", "--"}, untrack); err != nil {
		return err
	}
	return s.updateIndex(ctx, gitDir, cwd, []string{"add", "--"}, stage)
}

func (s *Store) updateIndex(ctx context.Context, gitDir, cwd string, prefix, paths []string) error {
	// Update in chunks so a large change set cannot overflow the arg limit.
	const chunk = 256
	for i := 0; i < len(paths); i += chunk {
		args := append(slices.Clone(prefix), paths[i:min(i+chunk, len(paths))]...)
		if _, err := s.git(ctx, gitDir, cwd, args...); err != nil {
			return err
		}
	}
	return nil
}
