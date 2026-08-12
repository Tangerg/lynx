package runs

import "sync"

// sessionRunChanges wakes application continuations after a committed Run
// lifecycle change. It carries no Run state: callers must re-read the durable
// projection after every wake.
type sessionRunChanges struct {
	mu       sync.Mutex
	sessions map[string]*sessionRunObservation
}

type sessionRunObservation struct {
	changed   chan struct{}
	observers int
}

func (changes *sessionRunChanges) observe(sessionID string) (<-chan struct{}, func()) {
	changes.mu.Lock()
	if changes.sessions == nil {
		changes.sessions = make(map[string]*sessionRunObservation)
	}
	observation := changes.sessions[sessionID]
	if observation == nil {
		observation = &sessionRunObservation{changed: make(chan struct{})}
		changes.sessions[sessionID] = observation
	}
	observation.observers++
	changes.mu.Unlock()

	var once sync.Once
	return observation.changed, func() {
		once.Do(func() {
			changes.mu.Lock()
			defer changes.mu.Unlock()
			observation.observers--
			if observation.observers == 0 && changes.sessions[sessionID] == observation {
				delete(changes.sessions, sessionID)
			}
		})
	}
}

func (changes *sessionRunChanges) notify(sessionID string) {
	changes.mu.Lock()
	defer changes.mu.Unlock()
	observation := changes.sessions[sessionID]
	if observation == nil {
		return
	}
	delete(changes.sessions, sessionID)
	close(observation.changed)
}
