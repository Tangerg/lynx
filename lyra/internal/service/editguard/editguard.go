// Package editguard enforces the read-before-edit invariant: a file must be
// READ before it is edited, and must not have CHANGED since (a user or a tool —
// e.g. a formatter — may have rewritten it), or the modification is refused with
// a message telling the agent to re-read. This is the reliability rule the
// mature Claude-optimized agent (Claude Code) relies on instead of a patch
// format: re-read rather than blindly clobber stale content.
//
// This package is the invariant ONLY — per-session read state, content hashing,
// and the verdict. Wrapping the read/edit/write TOOLS so the model hits it is
// presentation and lives in the engine's tool-assembly layer
// (internal/engine/toolset) — the LLM-facing counterpart to the wire translator.
package editguard

import (
	"crypto/sha256"
	"fmt"
	"os"
	"sync"
)

// Tracker records which files a session has read, and the content hash at read
// time, so an edit/write can be refused when the file was never read or has
// changed since. Keyed by session so one session reading a file doesn't license
// another to edit it. In-memory and per-engine: lost on restart (the agent just
// re-reads, exactly as Claude Code's per-session cache behaves). Content hash,
// not mtime — mtime is coarse and unreliable across filesystems, and the file
// content is read anyway. The zero value is not usable; build one with
// [NewTracker].
type Tracker struct {
	mu   sync.Mutex
	seen map[string]map[string]stamp // sessionID → absPath → stamp
}

type stamp struct {
	hash    [32]byte
	partial bool // only a line range was read → not safe to overwrite wholesale
}

func NewTracker() *Tracker {
	return &Tracker{seen: map[string]map[string]stamp{}}
}

// Record stamps abs as read by session, hashing its current content; partial
// marks a range-only read (a whole-file overwrite then needs a full read). A
// file that can't be hashed now is silently skipped — there's nothing to guard.
func (t *Tracker) Record(session, abs string, partial bool) {
	h, err := hashFile(abs)
	if err != nil {
		return
	}
	t.put(session, abs, stamp{hash: h, partial: partial})
}

// Refresh re-stamps abs from its current content (a full view), called after a
// successful edit/write so consecutive edits to the same file in a turn don't
// trip the guard.
func (t *Tracker) Refresh(session, abs string) {
	if h, err := hashFile(abs); err == nil {
		t.put(session, abs, stamp{hash: h})
	}
}

// Check reports whether abs may be modified by session. requireFull adds the
// partial-view rule (a whole-file overwrite needs a whole-file read). A file
// that can't be hashed now passes, so the underlying tool surfaces its own,
// more specific error.
func (t *Tracker) Check(session, abs string, requireFull bool) Result {
	st, ok := t.get(session, abs)
	if !ok {
		return resultMissing
	}
	cur, err := hashFile(abs)
	if err != nil {
		return resultOK
	}
	if cur != st.hash {
		return resultStale
	}
	if requireFull && st.partial {
		return resultPartial
	}
	return resultOK
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

// Result is the verdict of [Tracker.Check]: whether — and if not, why not — a
// file may be modified. Its zero value is the passing result.
type Result int

const (
	resultOK Result = iota
	resultMissing
	resultStale
	resultPartial
)

// Message renders the model-facing instruction for a non-passing result ("" when
// the file may be modified). verb is the blocked action ("editing" /
// "overwriting").
func (r Result) Message(path, verb string) string {
	switch r {
	case resultMissing:
		return fmt.Sprintf("You must read %s before %s it. Use the read tool first.", path, verb)
	case resultStale:
		return fmt.Sprintf("%s changed since you last read it (edited by the user or a tool). Read it again before %s it.", path, verb)
	case resultPartial:
		return fmt.Sprintf("You only read part of %s. Read the whole file before %s it.", path, verb)
	default:
		return ""
	}
}

func hashFile(abs string) ([32]byte, error) {
	data, err := os.ReadFile(abs)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(data), nil
}
