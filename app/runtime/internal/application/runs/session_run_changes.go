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

func (s *sessionRunChanges) observe(sessionID string) (<-chan struct{}, func()) {
	s.mu.Lock()
	if s.sessions == nil {
		s.sessions = make(map[string]*sessionRunObservation)
	}
	observation := s.sessions[sessionID]
	if observation == nil {
		observation = &sessionRunObservation{changed: make(chan struct{})}
		s.sessions[sessionID] = observation
	}
	observation.observers++
	s.mu.Unlock()

	var once sync.Once
	return observation.changed, func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			observation.observers--
			if observation.observers == 0 && s.sessions[sessionID] == observation {
				delete(s.sessions, sessionID)
			}
		})
	}
}

func (s *sessionRunChanges) notify(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	observation := s.sessions[sessionID]
	if observation == nil {
		return
	}
	delete(s.sessions, sessionID)
	close(observation.changed)
}
