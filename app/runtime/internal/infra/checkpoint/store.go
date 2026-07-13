package checkpoint

import (
	"os"
	"path/filepath"
	"sync"
)

// Store manages the shadow repos for every session. Safe for concurrent use:
// git ops are serialized by a per-WORKING-TREE mutex (locks), keyed by the
// canonical cwd — not the session id. The shadow GIT_DIR is per session, but the
// resource a git command mutates is the WORK TREE at cwd, and sessions can share
// one: a fork inherits its parent's cwd, and two sessions can open the same dir.
// A `reset --hard` (Restore) writing the tree must exclude another session's
// snapshot reading it, so the lock keys on the shared resource (the tree). A
// per-session key would let two sessions race one tree; it also still serializes
// a single session (one session → one cwd → one lock), which is what the async
// snapshot needs (a run's snapshot now runs off the run-finish path, so the next
// run can start before it finishes, and two git commands on one tree would
// otherwise race the index lock).
type Store struct {
	root  string   // base dir holding one shadow GIT_DIR per session
	locks sync.Map // canonical cwd → *sync.Mutex, serializing that work tree's git ops
}

// NewStore roots the shadow repos at dir (e.g. <LYRA_HOME>/checkpoints).
func NewStore(dir string) *Store { return &Store{root: dir} }

// lockFor returns the mutex serializing git ops on one working tree (keyed by
// the canonical cwd, the shared resource — see [Store]).
func (s *Store) lockFor(cwd string) *sync.Mutex {
	mu, _ := s.locks.LoadOrStore(cwd, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// DropSession removes a session's shadow repo (on session delete).
func (s *Store) DropSession(sessionID string) error {
	return os.RemoveAll(s.gitDir(sessionID))
}

func (s *Store) gitDir(sessionID string) string { return filepath.Join(s.root, sessionID) }
