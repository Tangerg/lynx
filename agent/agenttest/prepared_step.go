package agenttest

import (
	"context"
	"slices"
	"sync"

	agent "github.com/Tangerg/lynx/agent"
)

// PreparedStepRecorder is a concurrency-safe durability-boundary fixture. Each
// acknowledgment records its Snapshot and consumes one configured result;
// calls after the script is exhausted succeed. Its zero value is ready for use.
type PreparedStepRecorder struct {
	mu        sync.Mutex
	results   []error
	next      int
	snapshots []agent.Snapshot
}

// NewPreparedStepRecorder returns a recorder whose results are consumed in
// acknowledgment order. A nil result accepts that prepared boundary.
func NewPreparedStepRecorder(results ...error) *PreparedStepRecorder {
	return &PreparedStepRecorder{results: slices.Clone(results)}
}

// AcknowledgePreparedStep records snapshot and returns the next scripted result.
func (recorder *PreparedStepRecorder) AcknowledgePreparedStep(
	_ context.Context,
	snapshot agent.Snapshot,
) error {
	if recorder == nil {
		return nil
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.snapshots = append(recorder.snapshots, snapshot)
	if recorder.next >= len(recorder.results) {
		return nil
	}
	result := recorder.results[recorder.next]
	recorder.next++
	return result
}

// Snapshots returns prepared boundaries in acknowledgment order.
func (recorder *PreparedStepRecorder) Snapshots() []agent.Snapshot {
	if recorder == nil {
		return nil
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return slices.Clone(recorder.snapshots)
}
