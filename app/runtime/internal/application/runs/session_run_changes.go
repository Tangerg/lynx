package runs

import "sync"

// sessionRunChanges wakes application continuations after a committed Run
// lifecycle change. It carries no Run state: callers must re-read the durable
// projection after every wake.
type sessionRunChanges struct {
	mu       sync.Mutex
	sessions map[string]chan struct{}
}

func (changes *sessionRunChanges) observe(sessionID string) <-chan struct{} {
	changes.mu.Lock()
	defer changes.mu.Unlock()
	if changes.sessions == nil {
		changes.sessions = make(map[string]chan struct{})
	}
	changed := changes.sessions[sessionID]
	if changed == nil {
		changed = make(chan struct{})
		changes.sessions[sessionID] = changed
	}
	return changed
}

func (changes *sessionRunChanges) notify(sessionID string) {
	changes.mu.Lock()
	defer changes.mu.Unlock()
	if changes.sessions == nil {
		changes.sessions = make(map[string]chan struct{})
	}
	if changed := changes.sessions[sessionID]; changed != nil {
		close(changed)
	}
	changes.sessions[sessionID] = make(chan struct{})
}
