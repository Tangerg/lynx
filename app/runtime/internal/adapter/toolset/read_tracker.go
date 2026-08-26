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

func (r *readTracker) record(session, path string, fingerprint contentFingerprint) {
	r.put(session, path, readStamp{hash: fingerprint})
}

func (r *readTracker) refresh(session, path string, fingerprint contentFingerprint) {
	r.put(session, path, readStamp{hash: fingerprint})
}

func (r *readTracker) forget(session, path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	paths := r.seen[session]
	delete(paths, path)
	if len(paths) == 0 {
		delete(r.seen, session)
	}
}

func (r *readTracker) check(session, path string, current contentFingerprint) guardVerdict {
	st, ok := r.get(session, path)
	if !ok {
		return readRequired
	}
	if current != st.hash {
		return contentChanged
	}
	return mutationAllowed
}

func (r *readTracker) put(session, path string, st readStamp) {
	r.mu.Lock()
	defer r.mu.Unlock()
	paths := r.seen[session]
	if paths == nil {
		paths = map[string]readStamp{}
		r.seen[session] = paths
	}
	paths[path] = st
}

func (r *readTracker) get(session, path string) (readStamp, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.seen[session][path]
	return st, ok
}

type guardVerdict uint8

const (
	mutationAllowed guardVerdict = iota
	readRequired
	contentChanged
)

func (g guardVerdict) allowed() bool { return g == mutationAllowed }
