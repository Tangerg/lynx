package kernel

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

// Mid-run steering plumbing. A turn supplies a SteerSource on RunChatRequest;
// it travels two hops to reach the tool loop's BeforeRound hook:
//
//	StartChat → steerExtension (process-scope, not the serializable blackboard,
//	            since it's a live func) → runChatTurn resolves it via steerFrom
//	            → stashes it on the per-round context (withSteerSource) → the
//	            tool middleware's BeforeRound reads steerSourceFrom and drains it
//	            before each continuation round.
//
// The context hop (rather than relying on the platform to propagate the
// StartChat context's values into the action) makes the handoff a guarantee:
// runChatTurn owns the context it hands the stream.

// SteerSource yields user messages to inject into a running tool loop before
// the next model round. Returns nil when nothing is queued. Must not block.
type SteerSource func() []chat.Message

// steerExtension carries a turn's SteerSource as a process-scope extension —
// the same mechanism perRunChatClient / the tool observer use to reach the
// running action without going through the snapshot-able blackboard.
type steerExtension struct{ source SteerSource }

// Name implements [core.Extension]; unique across the process extension slice.
func (steerExtension) Name() string { return "lyra:steer-source" }

// steerFrom extracts the SteerSource a turn attached via StartChat, or nil.
func steerFrom(opts *core.ProcessOptions) SteerSource {
	if opts == nil {
		return nil
	}
	for _, ext := range opts.Extensions {
		if s, ok := ext.(steerExtension); ok {
			return s.source
		}
	}
	return nil
}

type steerContextKey struct{}

// withSteerSource stashes s on ctx so the tool loop's BeforeRound hook can
// reach it; steerSourceFrom reads it back (nil when unset).
func withSteerSource(ctx context.Context, s SteerSource) context.Context {
	return context.WithValue(ctx, steerContextKey{}, s)
}

func steerSourceFrom(ctx context.Context) SteerSource {
	s, _ := ctx.Value(steerContextKey{}).(SteerSource)
	return s
}
