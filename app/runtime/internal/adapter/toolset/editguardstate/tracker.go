// Package editguardstate tracks the read-before-edit state owned by the tool
// assembly. It has no meaning outside the file-mutating tools it protects.
package editguardstate

import (
	"crypto/sha256"
	"sync"
)

// Tracker records the file content each session has read. An edit or write is
// admitted only when the file is still at that content and, for a replacement,
// the session read the complete file.
type Tracker struct {
	mu   sync.Mutex
	seen map[string]map[string]stamp
}

type stamp struct {
	hash    Fingerprint
	partial bool
}

// Fingerprint identifies file content without coupling the guard to a
// filesystem implementation.
type Fingerprint [32]byte

// FingerprintOf returns the content identity used by the guard.
func FingerprintOf(content []byte) Fingerprint { return sha256.Sum256(content) }

// NewTracker creates a tracker for one tool environment.
func NewTracker() *Tracker {
	return &Tracker{seen: map[string]map[string]stamp{}}
}

// Record marks path as read by session. partial is true for a range-only read.
func (t *Tracker) Record(session, path string, fingerprint Fingerprint, partial bool) {
	t.put(session, path, stamp{hash: fingerprint, partial: partial})
}

// Refresh records the content after a successful full mutation.
func (t *Tracker) Refresh(session, path string, fingerprint Fingerprint) {
	t.put(session, path, stamp{hash: fingerprint})
}

// Check reports whether session may mutate path.
func (t *Tracker) Check(session, path string, current Fingerprint, requireFull bool) Result {
	st, ok := t.get(session, path)
	if !ok {
		return ResultReadRequired
	}
	if current != st.hash {
		return ResultChanged
	}
	if requireFull && st.partial {
		return ResultFullReadRequired
	}
	return ResultAllowed
}

func (t *Tracker) put(session, path string, st stamp) {
	t.mu.Lock()
	defer t.mu.Unlock()
	paths := t.seen[session]
	if paths == nil {
		paths = map[string]stamp{}
		t.seen[session] = paths
	}
	paths[path] = st
}

func (t *Tracker) get(session, path string) (stamp, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.seen[session][path]
	return st, ok
}

// Result is a structured mutation-admission outcome. Tool wording belongs to
// the enclosing toolset package.
type Result uint8

const (
	ResultAllowed Result = iota
	ResultReadRequired
	ResultChanged
	ResultFullReadRequired
)

// Allowed reports whether the mutation is admitted.
func (r Result) Allowed() bool { return r == ResultAllowed }
