package turn

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

// engineDep is the narrow behavioral surface turn depends on. It
// captures exactly the engine operations the turn dispatcher drives
// — async turn dispatch (plan mode included), steering, post-turn
// maintenance — and nothing more.
//
// StartTurn returns an [kernel.TurnProcess] that wraps the running
// [runtime.AgentProcess]; the dispatcher drives the turn off that
// handle (Done channel for completion, Status / Failure for
// terminal cause, Cancel for kill, Output for typed result) rather
// than a bare goroutine. This is the lyra-as-agent-best-practice
// pattern — every turn is a real agent process, addressable by id.
//
// It's unexported: turn's own dependency abstraction with no
// implementer outside this module — *kernel.Engine satisfies it
// implicitly, so nothing names it (tests pass a stub the same way).
//
// The shared parameter/result types live in package kernel — its I/O schema
// (RunTurnRequest, TurnOutput, TurnProcess) and the maintenance port results
// (CompactionResult, ExtractionResult). Importing them carries no concrete-type
// coupling; what we shed is the *kernel.Engine dependency, so turn can be
// unit-tested and the layering matches the architecture.
type engineDep interface {
	StartTurn(ctx context.Context, req kernel.RunTurnRequest) kernel.TurnProcess
	// RestoreTurn rebuilds a turn's agent process from a persisted
	// snapshot and re-parks it for Resume — the cross-restart resume seam
	// (the live process is gone after a backend restart). Returns a
	// re-parked [kernel.TurnProcess] ready for Resume(approved).
	RestoreTurn(ctx context.Context, processID string, req kernel.RestoreTurnRequest) (kernel.TurnProcess, error)
	InjectUserMessage(ctx context.Context, sessionID, text string) error
	MaybeCompact(ctx context.Context, sessionID string, preCompact func(context.Context) bool) (kernel.CompactionResult, error)
	MaybeExtract(ctx context.Context, sessionID, cwd string) (kernel.ExtractionResult, error)
}
