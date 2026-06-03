package chat

import (
	"context"

	"github.com/Tangerg/lynx/lyra/internal/engine"
)

// Engine is the narrow behavioral surface chat depends on. It
// captures exactly the engine operations the chat service drives
// — async chat dispatch (plan mode included), steering, post-turn
// maintenance — and nothing more.
//
// StartChat returns an [engine.ChatProcess] that wraps the running
// [runtime.AgentProcess]; the service drives the turn off that
// handle (Done channel for completion, Status / Failure for
// terminal cause, Cancel for kill, Output for typed result) rather
// than a bare goroutine. This is the lyra-as-agent-best-practice
// pattern — every turn is a real agent process, addressable by id.
//
// *engine.Engine satisfies this interface implicitly. Tests pass
// a stub that records calls without spinning up a real platform.
//
// The shared parameter/result types still live in package engine
// (RunChatRequest, ChatOutput, ToolObserver, ChatProcess) — those
// describe the engine's I/O schema and importing them does not
// create a concrete-type coupling. What we shed is the
// *engine.Engine dependency, so chat can be unit-tested and the
// layering matches the architecture (engine composes services,
// not the other way).
type Engine interface {
	StartChat(ctx context.Context, req engine.RunChatRequest) engine.ChatProcess
	// RestoreChat rebuilds a turn's agent process from a persisted
	// snapshot and re-parks it for Resume — the cross-restart resume seam
	// (the live process is gone after a backend restart). Returns a
	// re-parked [engine.ChatProcess] ready for Resume(approved).
	RestoreChat(ctx context.Context, processID string, req engine.RestoreChatRequest) (engine.ChatProcess, error)
	InjectUserMessage(ctx context.Context, sessionID, text string) error
	MaybeCompact(ctx context.Context, sessionID string) (engine.CompactionResult, error)
	MaybeExtract(ctx context.Context, sessionID string) (engine.ExtractionResult, error)
}
