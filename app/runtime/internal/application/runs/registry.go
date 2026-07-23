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

// Registry is the process-local registry of LIVE run segments. Session admission
// is owned separately by application/admission because Sessions and Runs share
// that invariant; durable run history lives in transcript.
//
// Its zero value is usable.
type Registry[P any] struct {
	mu   sync.Mutex
	runs map[string]Entry[P]
}

// Open registers an active run segment.
func (r *Registry[P]) Open(record Record, payload P) {
	r.mu.Lock()
	r.initLocked()
	r.runs[record.ID] = Entry[P]{Record: record, Payload: payload}
	r.mu.Unlock()
}

// Remove drops one completed segment and returns its former live entry.
func (r *Registry[P]) Remove(id string) (entry Entry[P], ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok = r.runs[id]
	if ok {
		delete(r.runs, id)
	}
	return entry, ok
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

func (r *Registry[P]) initLocked() {
	if r.runs == nil {
		r.runs = map[string]Entry[P]{}
	}
}
