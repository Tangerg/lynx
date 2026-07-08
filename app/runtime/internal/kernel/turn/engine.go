package turn

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

// turnStarter is the fresh-turn dispatch surface. StartTurn returns a
// [kernel.TurnProcess] that wraps the running [runtime.AgentProcess]; the
// dispatcher drives the turn off that handle (Done channel for completion,
// Status / Failure for terminal cause, Cancel for kill, Output for typed
// result) rather than a bare goroutine.
//
// This is the lyra-as-agent-best-practice pattern: every turn is a real agent
// process, addressable by id. The interface is unexported so turn owns its
// consumer-side dependency shape.
//
// The shared parameter/result types live in package kernel — its I/O schema
// (TurnRequest, TurnOutput, TurnProcess) and the maintenance port results
// (CompactionResult, ExtractionResult). Importing them carries no concrete-type
// coupling; what we shed is the *kernel.Engine dependency, so turn can be
// unit-tested and the layering matches the architecture.
type turnStarter interface {
	StartTurn(ctx context.Context, req kernel.TurnRequest) kernel.TurnProcess
}

// turnRestorer is the cross-restart resume seam. It rebuilds a turn's agent
// process from a persisted snapshot and re-parks it for Resume.
type turnRestorer interface {
	// RestoreTurn rebuilds a turn's agent process from a persisted
	// snapshot and re-parks it for Resume — the cross-restart resume seam
	// (the live process is gone after a backend restart). Returns a
	// re-parked [kernel.TurnProcess] ready for Resume(approved).
	RestoreTurn(ctx context.Context, processID string, req kernel.RestoreTurnRequest) (kernel.TurnProcess, error)
}

// steeringSink persists queued steering messages into chat history after a
// turn ends.
type steeringSink interface {
	InjectUserMessage(ctx context.Context, sessionID, text string) error
}

// maintenanceRunner owns post-turn maintenance hooks.
type maintenanceRunner interface {
	MaybeCompact(ctx context.Context, sessionID string, preCompact func(context.Context) bool) (kernel.CompactionResult, error)
	MaybeExtract(ctx context.Context, sessionID, cwd string) (kernel.ExtractionResult, error)
}
