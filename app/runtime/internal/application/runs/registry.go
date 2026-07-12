package runs

import (
	"maps"
	"slices"
	"sync"
	"time"
)

// Record is the observable state of an active run segment.
type Record struct {
	ID           string
	SessionID    string
	Cwd          string
	CreatedAt    time.Time
	TurnID       string
	ParentRunID  string
	Provider     string
	Model        string
	CancelReason string
}

// Entry pairs a run record with the payload the delivery layer attaches.
type Entry[P any] struct {
	Record  Record
	Payload P
}

// Registry is the process-local registry of LIVE run segments and their
// admission slots — the in-memory truth of "what is running right now" (durable
// run history lives in transcript). It is not a data bag: it enforces one writer
// per session — either one open run or one in-progress admission claim, never
// both — so run start / resume / destructive session mutation can't race.
//
// This is application-owned run-lifecycle state, held by [Coordinator] alongside
// the [Journal]. It is generic over the payload P the delivery layer attaches
// per entry (the run's cancel func + event journal), keeping the admission
// invariant here without pulling wire/executor types in.
//
// Its zero value is usable.
type Registry[P any] struct {
	mu       sync.Mutex
	runs     map[string]Entry[P]
	claiming map[string]struct{}
}

// ClaimSession reserves a session's single-writer slot for run admission.
func (r *Registry[P]) ClaimSession(sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.activeSessionLocked(sessionID) {
		return false
	}
	r.initLocked()
	r.claiming[sessionID] = struct{}{}
	return true
}

// ReleaseSession drops an admission claim.
func (r *Registry[P]) ReleaseSession(sessionID string) {
	r.mu.Lock()
	delete(r.claiming, sessionID)
	r.mu.Unlock()
}

// Open registers an active run segment.
func (r *Registry[P]) Open(record Record, payload P) {
	r.mu.Lock()
	r.initLocked()
	r.runs[record.ID] = Entry[P]{Record: record, Payload: payload}
	r.mu.Unlock()
}

// Close removes an active run segment.
func (r *Registry[P]) Close(id string) (Entry[P], bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.runs[id]
	if ok {
		delete(r.runs, id)
	}
	return e, ok
}

// Get returns an active run segment.
func (r *Registry[P]) Get(id string) (Entry[P], bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.runs[id]
	return e, ok
}

// Contains reports whether a run segment is active.
func (r *Registry[P]) Contains(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.runs[id]
	return ok
}

// MarkCancel records the human-facing cancel reason and returns the live run.
func (r *Registry[P]) MarkCancel(id, reason string) (Entry[P], bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.runs[id]
	if !ok {
		return Entry[P]{}, false
	}
	e.Record.CancelReason = reason
	r.runs[id] = e
	return e, true
}

// List snapshots active run segments.
func (r *Registry[P]) List() []Entry[P] {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Collect(maps.Values(r.runs))
}

// ActiveSession reports whether the session has an open run or admission claim.
func (r *Registry[P]) ActiveSession(sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activeSessionLocked(sessionID)
}

// ActiveSessions snapshots session ids with open runs or admission claims.
func (r *Registry[P]) ActiveSessions() map[string]bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	set := make(map[string]bool, len(r.runs)+len(r.claiming))
	for id := range r.claiming {
		set[id] = true
	}
	for _, e := range r.runs {
		set[e.Record.SessionID] = true
	}
	return set
}

// ActiveSessionWithCwd returns an active session using cwd.
func (r *Registry[P]) ActiveSessionWithCwd(cwd string) string {
	if cwd == "" {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.runs {
		if e.Record.Cwd == cwd {
			return e.Record.SessionID
		}
	}
	return ""
}

func (r *Registry[P]) activeSessionLocked(sessionID string) bool {
	if _, ok := r.claiming[sessionID]; ok {
		return true
	}
	for _, e := range r.runs {
		if e.Record.SessionID == sessionID {
			return true
		}
	}
	return false
}

func (r *Registry[P]) initLocked() {
	if r.runs == nil {
		r.runs = map[string]Entry[P]{}
	}
	if r.claiming == nil {
		r.claiming = map[string]struct{}{}
	}
}
