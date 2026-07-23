// Package queries is the application-owned read surface over a session's durable
// execution record: the transcript (items + runs) and open HITL interrupts.
// These are projections read directly from persistence (§5.4) — no aggregate is
// loaded and no command store is fattened with reads. Delivery drives them for
// items.list and interrupts.list.
package queries

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// TranscriptReader is the coordinator's view of the durable transcript
// projection: a session's item and run history.
type TranscriptReader interface {
	List(ctx context.Context, sessionID string) ([]transcript.Item, []transcript.Run, error)
}

// InterruptReader is the coordinator's view of the open-interrupt registry: a
// session's open interrupts, or every pending interrupt when sessionID is empty.
type InterruptReader interface {
	List(ctx context.Context, sessionID string) ([]interrupts.Pending, error)
}

// Coordinator serves the session read projections. Stateless beyond its store
// collaborators; safe to share.
type Coordinator struct {
	transcript TranscriptReader
	interrupts InterruptReader
}

// Dependencies is the collaborator set [New] wires into a Coordinator.
type Dependencies struct {
	Transcript TranscriptReader
	Interrupts InterruptReader
}

// New returns a query Coordinator over deps.
func New(deps Dependencies) *Coordinator {
	return &Coordinator{
		transcript: deps.Transcript,
		interrupts: deps.Interrupts,
	}
}

// ListTranscript returns a session's durable item history and run records.
func (c *Coordinator) ListTranscript(ctx context.Context, sessionID string) ([]transcript.Item, []transcript.Run, error) {
	return c.transcript.List(ctx, sessionID)
}

// ListPendingInterrupts returns durable open HITL interrupts. An empty sessionID
// returns every pending interrupt.
func (c *Coordinator) ListPendingInterrupts(ctx context.Context, sessionID string) ([]interrupts.Pending, error) {
	return c.interrupts.List(ctx, sessionID)
}
