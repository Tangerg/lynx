package session

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// Repo is the in-memory data layer that every concrete
// [Service] implementation builds on. It owns the session map +
// the lock; the mutate / read methods are pure (no persistence,
// no context). Persistent backends wrap a Repo and call
// [Repo.Snapshot] / [Repo.Restore] around it.
//
// Splitting this out of the in-process Service implementation
// lets storage-backed wrappers reuse the map / lock / lookup
// logic without re-implementing it line-for-line — the only
// thing they add on top is "write to disk after each mutate".
//
// All methods are safe for concurrent use.
type Repo struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewRepo returns an empty repo. The map is allocated up front so
// every method can hold the lock and probe / mutate without a
// nil check.
func NewRepo() *Repo {
	return &Repo{sessions: map[string]*Session{}}
}

// List returns a snapshot of every stored session in
// map-iteration order. Returned values are copies — callers can
// mutate without holding the lock.
func (r *Repo) List() []Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Session, 0, len(r.sessions))
	for _, sess := range r.sessions {
		out = append(out, *sess)
	}
	return out
}

// Get returns the session at id, or false when the id is unknown.
// Caller decides whether "missing" is an error.
func (r *Repo) Get(id string) (Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sess, ok := r.sessions[id]
	if !ok {
		return Session{}, false
	}
	return *sess, true
}

// Create inserts a fresh session and returns it. The new session
// has a generated UUID id; StartedAt + UpdatedAt are set to now.
func (r *Repo) Create(title string) Session {
	now := time.Now().UTC()
	sess := &Session{
		ID:        uuid.NewString(),
		Title:     title,
		StartedAt: now,
		UpdatedAt: now,
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[sess.ID] = sess
	return *sess
}

// Fork creates a child session pointing at parentID via
// ParentID, recording atMessageID in Metadata. Returns false when
// the parent is unknown — caller maps that to its public
// error type.
func (r *Repo) Fork(parentID, atMessageID string) (Session, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	parent, ok := r.sessions[parentID]
	if !ok {
		return Session{}, false
	}
	now := time.Now().UTC()
	child := &Session{
		ID:        uuid.NewString(),
		Title:     parent.Title + " (fork)",
		ParentID:  parentID,
		StartedAt: now,
		UpdatedAt: now,
		Metadata: map[string]string{
			"fork_at_message_id": atMessageID,
		},
	}
	r.sessions[child.ID] = child
	return *child, true
}

// Delete removes the session at id. Returns the removed session
// (for rollback) and a found flag. Idempotent — calling on an
// unknown id is not an error.
func (r *Repo) Delete(id string) (Session, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sess, ok := r.sessions[id]
	if !ok {
		return Session{}, false
	}
	removed := *sess
	delete(r.sessions, id)
	return removed, true
}

// Touch refreshes UpdatedAt and bumps TurnCount. Returns false
// when the id is unknown.
func (r *Repo) Touch(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	sess, ok := r.sessions[id]
	if !ok {
		return false
	}
	sess.UpdatedAt = time.Now().UTC()
	sess.TurnCount++
	return true
}

// Restore replaces the current contents with list, holding the
// write lock. Used by persistent backends to load on startup.
// Existing entries are dropped — partial restore is not
// supported.
func (r *Repo) Restore(list []Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions = make(map[string]*Session, len(list))
	for i := range list {
		sess := list[i]
		r.sessions[sess.ID] = &sess
	}
}

// Insert puts sess into the repo verbatim, replacing any
// existing entry with the same id. Used by persistent backends
// during rollback (re-inserting a session that Delete removed).
func (r *Repo) Insert(sess Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := sess
	r.sessions[sess.ID] = &clone
}
