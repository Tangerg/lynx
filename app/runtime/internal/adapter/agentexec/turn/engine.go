package turn

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
)

// engineDep is the turn dispatcher's single consumer-side view of
// [agentexec.Engine] — the one collaborator every turn drives. It is unexported so
// turn owns its dependency shape and can be unit-tested against a stub, and it
// is one cohesive interface (not per-verb sub-interfaces) because the dispatcher
// is the sole consumer and uses all of it.
//
// The shared parameter/result types live in package kernel — its I/O schema
// (TurnRequest, TurnOutput, TurnProcess) and the maintenance port results
// (CompactionResult, ExtractionResult). Importing them carries no concrete-type
// coupling; what we shed is the *agentexec.Engine dependency, so the layering
// matches the architecture.
//
// This is the lyra-as-agent-best-practice pattern: every turn is a real agent
// process, addressable by id. StartTurn returns a [agentexec.TurnProcess] that
// wraps the running [runtime.Process]; the dispatcher drives the turn off
// that handle (Done channel for completion, Status / Failure for terminal
// cause, Cancel for kill, Output for typed result) rather than a bare goroutine.
type engineDep interface {
	// StartTurn dispatches the underlying agent process for a fresh turn.
	StartTurn(ctx context.Context, request agentexec.TurnRequest) (agentexec.TurnProcess, error)

	// RestoreTurn rebuilds a turn's agent process from a persisted snapshot and
	// restores its Suspension for Resume — the cross-restart resume seam (the live process
	// is gone after a backend restart). Returns a waiting [agentexec.TurnProcess]
	// ready for Resume(approved).
	RestoreTurn(ctx context.Context, processID string, request agentexec.RestoreTurnRequest) (agentexec.TurnProcess, error)

	// InjectUserMessage persists queued steering messages into chat history
	// after a turn ends.
	InjectUserMessage(ctx context.Context, sessionID, text string) error

	// MaybeCompact runs the post-turn compaction maintenance hook.
	MaybeCompact(ctx context.Context, sessionID string, preCompact func(context.Context) bool) (agentexec.CompactionResult, error)

	// MaybeExtract runs the post-turn knowledge-extraction maintenance hook.
	MaybeExtract(ctx context.Context, sessionID, cwd string) (agentexec.ExtractionResult, error)
}
