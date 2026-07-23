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

// liveSegment is the coordinator's process-local state for a currently active
// run. The registry only ever manages run handles, so making it generic would
// hide its actual lifecycle ownership.
type liveSegment struct {
	record Record
	handle *handle
}

// registry is the process-local registry of live run segments. Session
// admission is owned separately by application/admission because Sessions and
// Runs share that invariant; durable run history lives in transcript.
//
// Its zero value is usable.
type registry struct {
	mu   sync.Mutex
	runs map[string]liveSegment
}

// Open registers an active run segment.
func (r *registry) Open(record Record, handle *handle) {
	r.mu.Lock()
	r.initLocked()
	r.runs[record.ID] = liveSegment{record: record, handle: handle}
	r.mu.Unlock()
}

// Remove drops one completed segment and returns its former live state.
func (r *registry) Remove(id string) (segment liveSegment, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	segment, ok = r.runs[id]
	if ok {
		delete(r.runs, id)
	}
	return segment, ok
}

// Get returns an active run segment.
func (r *registry) Get(id string) (liveSegment, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	segment, ok := r.runs[id]
	return segment, ok
}

// Contains reports whether a run segment is active.
func (r *registry) Contains(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.runs[id]
	return ok
}

// MarkCancel records the human-facing cancel reason and returns the live run.
func (r *registry) MarkCancel(id, reason string) (liveSegment, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	segment, ok := r.runs[id]
	if !ok {
		return liveSegment{}, false
	}
	segment.record.CancelReason = reason
	r.runs[id] = segment
	return segment, true
}

// List snapshots active run records.
func (r *registry) List() []Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Record, 0, len(r.runs))
	for _, segment := range r.runs {
		out = append(out, segment.record)
	}
	slices.SortFunc(out, func(left, right Record) int {
		if order := left.CreatedAt.Compare(right.CreatedAt); order != 0 {
			return order
		}
		return strings.Compare(left.ID, right.ID)
	})
	return out
}

func (r *registry) initLocked() {
	if r.runs == nil {
		r.runs = map[string]liveSegment{}
	}
}
