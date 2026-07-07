package checkpoint

import "context"

// Restore resets cwd's work tree to the runID snapshot: tracked files revert,
// and files created-and-staged since (small enough to be tracked) are removed
// by the reset. Ignored files AND oversize files (never staged — see
// [maxCheckpointFileSize]) are left untouched: they're untracked, so the reset
// doesn't reach them, and we deliberately do NOT `git clean` (that would delete
// the user's large/ignored files the checkpoint never owned). The pre-restore
// state is captured as a commit first (when it differs from HEAD) so the restore
// is itself reversible (unrevert). Returns ErrUnavailable when runID has no
// snapshot.
func (s *Store) Restore(ctx context.Context, sessionID, cwd, runID string) error {
	mu := s.lockFor(cwd)
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
