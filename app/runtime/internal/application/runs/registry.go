package runs

import (
	"sync"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

// Record is the observable state of an active run segment.
type Record struct {
	ID             string
	SegmentID      string
	SessionID      string
	CWD            string
	CreatedAt      time.Time
	ExecutorID     string
	ModelSelection modelref.Selection
	// Capabilities is the Run's frozen optional behavior, carried on the live
	// record so an insufficient subscriber is refused before attachment.
	Capabilities run.Capabilities
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
	r.runs[record.ID] = liveSegment{record: cloneRecord(record), handle: handle}
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
	segment.record = cloneRecord(segment.record)
	return segment, ok
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
	segment.record = cloneRecord(segment.record)
	return segment, true
}

func cloneRecord(record Record) Record {
	record.Capabilities = record.Capabilities.Clone()
	return record
}

func (r *registry) initLocked() {
	if r.runs == nil {
		r.runs = map[string]liveSegment{}
	}
}
