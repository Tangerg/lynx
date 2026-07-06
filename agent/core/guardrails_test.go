package core_test

import (
	"context"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

func TestGuardrails_Empty(t *testing.T) {
	if !(*core.Guardrails)(nil).Empty() {
		t.Error("nil Guardrails: want Empty()=true")
	}
	if !(&core.Guardrails{}).Empty() {
		t.Error("zero-value Guardrails: want Empty()=true")
	}

	g := &core.Guardrails{
		CallMiddlewares: []chat.CallMiddleware{passthroughCall},
	}
	if g.Empty() {
		t.Error("Guardrails with CallMiddlewares: want Empty()=false")
	}

	g2 := &core.Guardrails{
		StreamMiddlewares: []chat.StreamMiddleware{passthroughStream},
	}
	if g2.Empty() {
		t.Error("Guardrails with StreamMiddlewares: want Empty()=false")
	}
}

func TestGuardrails_MiddlewareChain(t *testing.T) {
	if chain := (*core.Guardrails)(nil).MiddlewareChain(); len(chain.CallMiddlewares()) != 0 || len(chain.StreamMiddlewares()) != 0 {
		t.Errorf("nil Guardrails: want empty chain, got call=%d stream=%d", len(chain.CallMiddlewares()), len(chain.StreamMiddlewares()))
	}
	if chain := (&core.Guardrails{}).MiddlewareChain(); len(chain.CallMiddlewares()) != 0 || len(chain.StreamMiddlewares()) != 0 {
		t.Errorf("zero-value: want empty chain, got call=%d stream=%d", len(chain.CallMiddlewares()), len(chain.StreamMiddlewares()))
	}

	g := &core.Guardrails{
		CallMiddlewares:   []chat.CallMiddleware{passthroughCall, passthroughCall},
		StreamMiddlewares: []chat.StreamMiddleware{passthroughStream},
	}
	chain := g.MiddlewareChain()
	if got := len(chain.CallMiddlewares()); got != 2 {
		t.Fatalf("call middlewares: want 2, got %d", got)
	}
	if got := len(chain.StreamMiddlewares()); got != 1 {
		t.Fatalf("stream middlewares: want 1, got %d", got)
	}
}

func passthroughCall(next chat.CallHandler) chat.CallHandler { return next }
func passthroughStream(next chat.StreamHandler) chat.StreamHandler {
	return chat.StreamHandlerFunc(func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
		return next.Stream(ctx, req)
	})
}
