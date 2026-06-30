package turn

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

// engineDep is the narrow behavioral surface turn depends on. It
// captures exactly the engine operations the turn service drives
// — async chat dispatch (plan mode included), steering, post-turn
// maintenance — and nothing more.
//
// StartChat returns an [kernel.ChatProcess] that wraps the running
// [runtime.AgentProcess]; the service drives the turn off that
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
// (RunChatRequest, ChatOutput, ChatProcess) and the maintenance port results
// (CompactionResult, ExtractionResult). Importing them carries no concrete-type
// coupling; what we shed is the *kernel.Engine dependency, so turn can be
// unit-tested and the layering matches the architecture.
type engineDep interface {
	StartChat(ctx context.Context, req kernel.RunChatRequest) kernel.ChatProcess
	// RestoreChat rebuilds a turn's agent process from a persisted
	// snapshot and re-parks it for Resume — the cross-restart resume seam
	// (the live process is gone after a backend restart). Returns a
	// re-parked [kernel.ChatProcess] ready for Resume(approved).
	RestoreChat(ctx context.Context, processID string, req kernel.RestoreChatRequest) (kernel.ChatProcess, error)
	InjectUserMessage(ctx context.Context, sessionID, text string) error
	MaybeCompact(ctx context.Context, sessionID string, preCompact func(context.Context) bool) (kernel.CompactionResult, error)
	MaybeExtract(ctx context.Context, sessionID, cwd string) (kernel.ExtractionResult, error)
}
