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

func TestGuardrails_MiddlewareValues(t *testing.T) {
	if v := (*core.Guardrails)(nil).MiddlewareValues(); v != nil {
		t.Errorf("nil Guardrails: want nil values, got %v", v)
	}
	if v := (&core.Guardrails{}).MiddlewareValues(); v != nil {
		t.Errorf("zero-value: want nil values, got %v", v)
	}

	g := &core.Guardrails{
		CallMiddlewares:   []chat.CallMiddleware{passthroughCall, passthroughCall},
		StreamMiddlewares: []chat.StreamMiddleware{passthroughStream},
	}
	values := g.MiddlewareValues()
	if len(values) != 3 {
		t.Fatalf("interleaved values: want 3, got %d", len(values))
	}
}

func passthroughCall(next chat.CallHandler) chat.CallHandler { return next }
func passthroughStream(next chat.StreamHandler) chat.StreamHandler {
	return chat.StreamHandlerFunc(func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
		return next.Stream(ctx, req)
	})
}
