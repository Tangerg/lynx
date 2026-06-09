package runtime

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/a2a"
	"github.com/Tangerg/lynx/lyra/internal/engine"
)

// a2aAgent adapts the engine's one-shot chat turn to the [a2a.Agent] the
// A2A server bridge expects: an inbound A2A message (flattened to text by
// the executor) runs as a fresh chat turn, and the assistant's reply is
// yielded as a single chunk. Each call is independent — no session is bound,
// so a remote caller gets a clean turn against the engine's default workdir.
//
// One-shot (yield once) rather than token-streaming: the executor maps the
// chunk(s) onto the A2A task lifecycle either way, and the engine's blocking
// RunChat is the simplest faithful bridge. Token-level streaming would adapt
// the engine's observer and is a follow-up.
type a2aAgent struct {
	engine *engine.Engine
}

var _ a2a.Agent = a2aAgent{}

func (a a2aAgent) Run(ctx context.Context, input string) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		out, err := a.engine.RunChat(ctx, engine.RunChatRequest{Message: input})
		if err != nil {
			yield("", err)
			return
		}
		yield(out.Reply, nil)
	}
}

// A2AAgent exposes this runtime as an [a2a.Agent] so a transport can serve
// it over the A2A protocol (see [a2a.NewHTTPHandler]). The returned adapter
// runs each inbound message as an independent, sessionless chat turn.
func (r *Runtime) A2AAgent() a2a.Agent { return a2aAgent{engine: r.engine} }
