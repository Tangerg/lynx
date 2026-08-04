package toolset

import (
	"crypto/sha256"
	"sync"
)

// readTracker records the file content each session has read. A mutation is
// admitted only while the file still has that content.
type readTracker struct {
	mu   sync.Mutex
	seen map[string]map[string]readStamp
}

type readStamp struct {
	hash contentFingerprint
}

type contentFingerprint [32]byte

func fingerprintOf(content []byte) contentFingerprint { return sha256.Sum256(content) }

func newReadTracker() *readTracker {
	return &readTracker{seen: map[string]map[string]readStamp{}}
}

func (t *readTracker) record(session, path string, fingerprint contentFingerprint) {
	t.put(session, path, readStamp{hash: fingerprint})
}

func (t *readTracker) refresh(session, path string, fingerprint contentFingerprint) {
	t.put(session, path, readStamp{hash: fingerprint})
}

func (t *readTracker) check(session, path string, current contentFingerprint) guardVerdict {
	st, ok := t.get(session, path)
	if !ok {
		return readRequired
	}
	if current != st.hash {
		return contentChanged
	}
	return mutationAllowed
}

func (t *readTracker) put(session, path string, st readStamp) {
	t.mu.Lock()
	defer t.mu.Unlock()
	paths := t.seen[session]
	if paths == nil {
		paths = map[string]readStamp{}
		t.seen[session] = paths
	}
	paths[path] = st
}

func (t *readTracker) get(session, path string) (readStamp, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.seen[session][path]
	return st, ok
}

type guardVerdict uint8

const (
	mutationAllowed guardVerdict = iota
	readRequired
	contentChanged
)

func (r guardVerdict) allowed() bool { return r == mutationAllowed }
