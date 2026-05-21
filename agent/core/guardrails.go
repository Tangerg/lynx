package core

import "github.com/Tangerg/lynx/core/model/chat"

// Guardrails carries platform-wide chat middlewares that wrap every
// LLM call action bodies make through [ProcessContext.Chat] or
// [ProcessContext.ChatWithActionTools]. Use it to install global
// safety / logging / quota policy without sprinkling middleware
// registration through every action site.
//
// The values are plain [chat.CallMiddleware] / [chat.StreamMiddleware]
// — typically constructed via the helpers in `core/model/chat/middleware`
// (NewLoggerMiddleware, NewSafeguardMiddleware) but any user-supplied
// middleware satisfies the slot. Order is outer-first: the first entry
// sees a request earliest and a response latest, mirroring how the
// onion of middleware is composed at the chat client.
//
// Empty slices are valid — a Guardrails with no middlewares is a no-op
// pass-through. A nil *Guardrails is treated identically.
type Guardrails struct {
	// CallMiddlewares wrap blocking [chat.CallHandler] invocations.
	CallMiddlewares []chat.CallMiddleware

	// StreamMiddlewares wrap streaming [chat.StreamHandler] invocations.
	StreamMiddlewares []chat.StreamMiddleware
}

// Empty reports whether the guardrails would do anything when
// applied — `nil` or "both slices empty" both qualify.
func (g *Guardrails) Empty() bool {
	if g == nil {
		return true
	}
	return len(g.CallMiddlewares) == 0 && len(g.StreamMiddlewares) == 0
}

// MiddlewareValues returns the call + stream middlewares interleaved
// into the loose `[]any` slot that [chat.ClientRequest.WithMiddlewares]
// accepts. Returns nil when [Guardrails.Empty] is true so callers can
// pass the result through unconditionally.
func (g *Guardrails) MiddlewareValues() []any {
	if g.Empty() {
		return nil
	}
	out := make([]any, 0, len(g.CallMiddlewares)+len(g.StreamMiddlewares))
	for _, mw := range g.CallMiddlewares {
		out = append(out, mw)
	}
	for _, mw := range g.StreamMiddlewares {
		out = append(out, mw)
	}
	return out
}
