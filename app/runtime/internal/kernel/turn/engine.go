package turn

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

// engineDep is the turn dispatcher's single consumer-side view of
// [kernel.Engine] — the one collaborator every turn drives. It is unexported so
// turn owns its dependency shape and can be unit-tested against a stub, and it
// is one cohesive interface (not per-verb sub-interfaces) because the dispatcher
// is the sole consumer and uses all of it.
//
// The shared parameter/result types live in package kernel — its I/O schema
// (TurnRequest, TurnOutput, TurnProcess) and the maintenance port results
// (CompactionResult, ExtractionResult). Importing them carries no concrete-type
// coupling; what we shed is the *kernel.Engine dependency, so the layering
// matches the architecture.
//
// This is the lyra-as-agent-best-practice pattern: every turn is a real agent
// process, addressable by id. StartTurn returns a [kernel.TurnProcess] that
// wraps the running [runtime.AgentProcess]; the dispatcher drives the turn off
// that handle (Done channel for completion, Status / Failure for terminal
// cause, Cancel for kill, Output for typed result) rather than a bare goroutine.
type engineDep interface {
	// StartTurn dispatches the underlying agent process for a fresh turn.
	StartTurn(ctx context.Context, req kernel.TurnRequest) kernel.TurnProcess

	// RestoreTurn rebuilds a turn's agent process from a persisted snapshot and
	// re-parks it for Resume — the cross-restart resume seam (the live process
	// is gone after a backend restart). Returns a re-parked [kernel.TurnProcess]
	// ready for Resume(approved).
	RestoreTurn(ctx context.Context, processID string, req kernel.RestoreTurnRequest) (kernel.TurnProcess, error)

	// InjectUserMessage persists queued steering messages into chat history
	// after a turn ends.
	InjectUserMessage(ctx context.Context, sessionID, text string) error

	// MaybeCompact runs the post-turn compaction maintenance hook.
	MaybeCompact(ctx context.Context, sessionID string, preCompact func(context.Context) bool) (kernel.CompactionResult, error)

	// MaybeExtract runs the post-turn knowledge-extraction maintenance hook.
	MaybeExtract(ctx context.Context, sessionID, cwd string) (kernel.ExtractionResult, error)
}
