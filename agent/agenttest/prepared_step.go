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
func (p *PreparedStepRecorder) AcknowledgePreparedStep(
	_ context.Context,
	snapshot agent.Snapshot,
) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.snapshots = append(p.snapshots, snapshot)
	if p.next >= len(p.results) {
		return nil
	}
	result := p.results[p.next]
	p.next++
	return result
}

// Snapshots returns prepared boundaries in acknowledgment order.
func (p *PreparedStepRecorder) Snapshots() []agent.Snapshot {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.snapshots)
}
