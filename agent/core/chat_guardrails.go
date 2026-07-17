package core

import "github.com/Tangerg/lynx/core/chat"

// ChatGuardrails carries engine-wide call and stream middleware plus the bounded
// tool-loop policy used by Prompt. Runtime constructors snapshot the value and
// both middleware slices before retaining it. Middleware remains ordinary target
// core/chat composition; executable tools and loop state never enter a request.
type ChatGuardrails struct {
	CallMiddlewares   []chat.CallMiddleware
	StreamMiddlewares []chat.StreamMiddleware

	// MaxToolRounds bounds Prompt tool execution. Zero selects the
	// toolloop.Runner default.
	MaxToolRounds int
}

// Empty reports whether g changes chat execution.
func (g *ChatGuardrails) Empty() bool {
	if g == nil {
		return true
	}
	return len(g.CallMiddlewares) == 0 && len(g.StreamMiddlewares) == 0 && g.MaxToolRounds == 0
}
