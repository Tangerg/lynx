package runs

import (
	"sync"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
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
// run. The registry only ever manages Run-tree owners, so making it generic would
// hide its actual lifecycle ownership.
type liveSegment struct {
	record Record
	owner  *runTreeOwner
}

// registry is the process-local registry of live run segments. Session
// admission is owned separately by application/sessionadmission because Sessions and
// Runs share that invariant; durable run history lives in transcript.
//
// Its zero value is usable.
type registry struct {
	mu   sync.Mutex
	runs map[string]liveSegment
}

// Open registers an active run segment.
func (r *registry) Open(record Record, owner *runTreeOwner) {
	r.mu.Lock()
	r.initLocked()
	r.runs[record.ID] = liveSegment{record: cloneRecord(record), owner: owner}
	r.mu.Unlock()
}

// RemoveSegment drops one exact completed segment and returns its former live
// state. A Run can resume onto a replacement Segment as soon as terminal
// maintenance releases admission; an older pump must never delete that newer
// registry entry merely because both Segments share the same Run ID.
func (r *registry) RemoveSegment(id, segmentID string) (segment liveSegment, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	segment, ok = r.runs[id]
	if !ok || segment.record.SegmentID != segmentID {
		return liveSegment{}, false
	}
	delete(r.runs, id)
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
