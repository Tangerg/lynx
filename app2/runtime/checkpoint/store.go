package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store manages every session's shadow repository. It has two explicit
// synchronization domains: treeLocks serialize Git commands that inspect or
// mutate one shared working tree, while repoLocks serialize the lifecycle of one
// session's GIT_DIR (including DropSession). Snapshot and Restore always acquire
// tree then repository; DropSession touches only the repository, so the order is
// deadlock-free. Run lifecycle separately keeps same-session admission held
// until a terminal Snapshot returns, preventing the next run from crossing its
// own checkpoint boundary.
type Store struct {
	root      string   // base dir holding one shadow GIT_DIR per session
	treeLocks sync.Map // canonical cwd → *sync.Mutex, serializing that working tree
	repoLocks sync.Map // session id → *sync.Mutex, serializing one shadow repository
}

// NewStore roots the shadow repos at dir (e.g. <LYRA_HOME>/checkpoints).
func NewStore(dir string) *Store { return &Store{root: dir} }

func (s *Store) treeLockFor(cwd string) *sync.Mutex {
	mu, _ := s.treeLocks.LoadOrStore(cwd, &sync.Mutex{})
	return storedMutex(mu, "checkpoint tree lock")
}

func (s *Store) repoLockFor(sessionID string) *sync.Mutex {
	mu, _ := s.repoLocks.LoadOrStore(sessionID, &sync.Mutex{})
	return storedMutex(mu, "checkpoint repository lock")
}

func storedMutex(value any, owner string) *sync.Mutex {
	mu, ok := value.(*sync.Mutex)
	if !ok {
		panic(owner + " map contains a non-mutex value")
	}
	return mu
}

// DropSession removes a session's shadow repo (on session delete).
func (s *Store) DropSession(sessionID string) error {
	mu := s.repoLockFor(sessionID)
	mu.Lock()
	defer mu.Unlock()
	return os.RemoveAll(s.gitDir(sessionID))
}

// DropRuns removes the durable boundary refs owned by Runs that a committed
// history rollback deleted. The shadow repository is serialized with Snapshot,
// Restore and DropSession, so a finishing Run cannot recreate a discarded tag
// after the Session transaction has settled.
func (s *Store) DropRuns(ctx context.Context, sessionID string, runIDs []string) error {
	if len(runIDs) == 0 {
		return nil
	}
	mu := s.repoLockFor(sessionID)
	mu.Lock()
	defer mu.Unlock()

	gitDir := s.gitDir(sessionID)
	if _, err := os.Stat(filepath.Join(gitDir, "HEAD")); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("checkpoint: inspect session repository: %w", err)
	}
	for _, runID := range runIDs {
		if _, err := s.git(ctx, gitDir, "", "update-ref", "-d", "refs/tags/"+tagFor(runID)); err != nil {
			return fmt.Errorf("checkpoint: drop run %q: %w", runID, err)
		}
	}
	return nil
}

func (s *Store) gitDir(sessionID string) string { return filepath.Join(s.root, sessionID) }
