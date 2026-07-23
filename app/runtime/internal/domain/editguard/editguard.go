// Package editguard enforces the read-before-edit invariant: a file must be
// READ before it is edited, and must not have CHANGED since (a user or a tool —
// e.g. a formatter — may have rewritten it), or the modification is refused.
// This is the reliability rule mature
// coding agents rely on instead of a patch format: re-read rather than blindly
// clobber stale content.
//
// This package is the invariant ONLY — per-session read state, content hashing,
// and the verdict. Wrapping the read/edit/write TOOLS so the model hits it is
// presentation and lives in the kernel's tool-assembly layer
// (internal/adapter/toolset) — the LLM-facing counterpart to the wire translator.
package editguard

import (
	"crypto/sha256"
	"sync"
)

// Tracker records which files a session has read, and the content hash at read
// time, so an edit/write can be refused when the file was never read or has
// changed since. Keyed by session so one session reading a file doesn't license
// another to edit it. In-memory and per-engine: lost on restart (the agent just
// re-reads). Content hash, not mtime — mtime is coarse and unreliable across
// filesystems, and the file content is read anyway. The zero value is not
// usable; build one with [NewTracker].
type Tracker struct {
	mu   sync.Mutex
	seen map[string]map[string]stamp // sessionID → absPath → stamp
}

type stamp struct {
	hash    Fingerprint
	partial bool // only a line range was read → not safe to overwrite wholesale
}

// Fingerprint is a content identity supplied by the filesystem adapter. The
// domain compares fingerprints; it never opens files itself.
type Fingerprint [32]byte

// FingerprintOf computes the stable content identity used by the guard.
func FingerprintOf(content []byte) Fingerprint { return sha256.Sum256(content) }

func NewTracker() *Tracker {
	return &Tracker{seen: map[string]map[string]stamp{}}
}

// Record stamps path as read by session. partial marks a range-only read (a
// whole-file overwrite then needs a full read).
func (t *Tracker) Record(session, path string, fingerprint Fingerprint, partial bool) {
	t.put(session, path, stamp{hash: fingerprint, partial: partial})
}

// Refresh re-stamps path from its current content (a full view), called after a
// successful edit/write so consecutive edits to the same file in a turn don't
// trip the guard.
func (t *Tracker) Refresh(session, path string, fingerprint Fingerprint) {
	t.put(session, path, stamp{hash: fingerprint})
}

// Check reports whether path may be modified by session. requireFull adds the
// partial-view rule (a whole-file overwrite needs a whole-file read).
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

func (t *Tracker) put(session, abs string, st stamp) {
	t.mu.Lock()
	defer t.mu.Unlock()
	m := t.seen[session]
	if m == nil {
		m = map[string]stamp{}
		t.seen[session] = m
	}
	m[abs] = st
}

func (t *Tracker) get(session, abs string) (stamp, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.seen[session][abs]
	return st, ok
}

// Result is the structured verdict of [Tracker.Check]. It carries no model or
// tool wording; the tool adapter decides how to present a refusal.
type Result int

const (
	// ResultAllowed permits the mutation.
	ResultAllowed Result = iota
	// ResultReadRequired means the session has not read the file.
	ResultReadRequired
	// ResultChanged means the file differs from the last read fingerprint.
	ResultChanged
	// ResultFullReadRequired means only a partial read is recorded.
	ResultFullReadRequired
)

// Allowed reports whether the mutation may proceed.
func (r Result) Allowed() bool { return r == ResultAllowed }
