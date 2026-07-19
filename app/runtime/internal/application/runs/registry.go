package runs

import (
	"slices"
	"strings"
	"sync"
	"time"
)

// Record is the observable state of an active run segment.
type Record struct {
	ID           string
	SegmentID    string
	SessionID    string
	Cwd          string
	CreatedAt    time.Time
	TurnID       string
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
	mu          sync.Mutex
	runs        map[string]Entry[P]
	claims      map[string]map[uint64]struct{}
	nextClaimID uint64
}

// AcquireSession reserves a session's single-writer slot for run admission.
// The returned release owns one specific claim: releasing an older admission
// can never erase a newer terminal-maintenance claim for the same session.
func (r *Registry[P]) AcquireSession(sessionID string) (release func(), ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.activeSessionLocked(sessionID) {
		return nil, false
	}
	return r.addClaimLocked(sessionID), true
}

// Open registers an active run segment.
func (r *Registry[P]) Open(record Record, payload P) {
	r.mu.Lock()
	r.initLocked()
	r.runs[record.ID] = Entry[P]{Record: record, Payload: payload}
	r.mu.Unlock()
}

// BeginMaintenance atomically removes a completed segment and acquires a
// separately-owned session claim. The caller must invoke release after boundary
// maintenance completes. Keeping this claim distinct from the opening admission
// prevents a fast run's delayed opening release from erasing the maintenance
// fence. This closes the admission gap in which a new run or destructive session
// mutation could otherwise race terminal cleanup.
func (r *Registry[P]) BeginMaintenance(id string) (entry Entry[P], release func(), ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.runs[id]
	if ok {
		delete(r.runs, id)
		release = r.addClaimLocked(e.Record.SessionID)
	}
	return e, release, ok
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
	out := make([]Entry[P], 0, len(r.runs))
	for _, entry := range r.runs {
		out = append(out, entry)
	}
	slices.SortFunc(out, func(left, right Entry[P]) int {
		if order := left.Record.CreatedAt.Compare(right.Record.CreatedAt); order != 0 {
			return order
		}
		return strings.Compare(left.Record.ID, right.Record.ID)
	})
	return out
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
	set := make(map[string]bool, len(r.runs)+len(r.claims))
	for id := range r.claims {
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
	if len(r.claims[sessionID]) > 0 {
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
	if r.claims == nil {
		r.claims = map[string]map[uint64]struct{}{}
	}
}

func (r *Registry[P]) addClaimLocked(sessionID string) func() {
	r.initLocked()
	r.nextClaimID++
	id := r.nextClaimID
	owners := r.claims[sessionID]
	if owners == nil {
		owners = map[uint64]struct{}{}
		r.claims[sessionID] = owners
	}
	owners[id] = struct{}{}

	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			owners := r.claims[sessionID]
			delete(owners, id)
			if len(owners) == 0 {
				delete(r.claims, sessionID)
			}
		})
	}
}
