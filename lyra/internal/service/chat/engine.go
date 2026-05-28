package chat

import (
	"context"

	"github.com/Tangerg/lynx/lyra/internal/engine"
)

// Engine is the narrow behavioural surface chat depends on. It
// captures exactly the engine operations the chat service drives
// — RunChat / GeneratePlan / steering / post-turn maintenance —
// and nothing more.
//
// *engine.Engine satisfies this interface implicitly. Tests pass
// a stub that records calls without spinning up a real platform.
//
// The shared parameter/result types still live in package engine
// (RunChatRequest, ChatOutput, ToolObserver) — those describe the
// engine's I/O schema and importing them does not create a
// concrete-type coupling. What we shed is the *engine.Engine
// dependency, so chat can be unit-tested and the layering matches
// the architecture (engine composes services, not the other way).
type Engine interface {
	RunChat(ctx context.Context, req engine.RunChatRequest) (engine.ChatOutput, error)
	GeneratePlan(ctx context.Context, userMessage string) (string, error)
	InjectUserMessage(ctx context.Context, sessionID, text string) error
	MaybeCompact(ctx context.Context, sessionID string) (bool, error)
	MaybeExtract(ctx context.Context, sessionID string) error
}
