package hitl_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/core/model/chat"
)

// fakeTool is a chat.CallableTool stub. Its Call records the
// arguments it received and returns a configured (text, err) pair.
type fakeTool struct {
	def      chat.ToolDefinition
	called   bool
	gotArgs  string
	resp     string
	respErr  error
}

func (t *fakeTool) Definition() chat.ToolDefinition { return t.def }
func (t *fakeTool) Metadata() chat.ToolMetadata     { return chat.ToolMetadata{} }
func (t *fakeTool) Call(_ context.Context, arguments string) (string, error) {
	t.called = true
	t.gotArgs = arguments
	return t.resp, t.respErr
}

func newFake(name, resp string) *fakeTool {
	return &fakeTool{
		def:  chat.ToolDefinition{Name: name, InputSchema: `{"type":"object"}`},
		resp: resp,
	}
}

func TestWithAwaiting_NilDeciderResultDelegates(t *testing.T) {
	inner := newFake("search", "result")
	wrapped := hitl.WithAwaiting(inner, func(context.Context, string) core.Awaitable { return nil })

	out, err := wrapped.Call(t.Context(), `{"q":"foo"}`)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if out != "result" {
		t.Fatalf("expected delegate result, got %q", out)
	}
	if !inner.called || inner.gotArgs != `{"q":"foo"}` {
		t.Fatalf("delegate not called with original args: called=%v args=%q", inner.called, inner.gotArgs)
	}
}

func TestWithAwaiting_NonNilDeciderReturnsPauseError(t *testing.T) {
	inner := newFake("search", "result")
	awaitable := hitl.NewConfirmation("approve search?", func(bool) core.ResponseImpact {
		return core.ResponseImpactUnchanged
	})

	wrapped := hitl.WithAwaiting(inner, func(context.Context, string) core.Awaitable { return awaitable })

	_, err := wrapped.Call(t.Context(), `{"q":"foo"}`)
	if err == nil {
		t.Fatal("expected PauseError, got nil")
	}

	pe, ok := errors.AsType[*hitl.PauseError](err)
	if !ok {
		t.Fatalf("expected *hitl.PauseError, got %T", err)
	}
	if pe.Request.ID() != awaitable.ID() {
		t.Fatalf("PauseError.Request id = %q, want %q", pe.Request.ID(), awaitable.ID())
	}
	if inner.called {
		t.Fatal("delegate must not run when decider returns Awaitable")
	}
}

func TestWithConfirmation_PromptsAndOnResponseFires(t *testing.T) {
	inner := newFake("delete", "deleted")

	var capturedMsg string
	var captured bool
	wrapped := hitl.WithConfirmation(
		inner,
		func(args string) string {
			return "confirm delete: " + args
		},
		func(approved bool) core.ResponseImpact {
			captured = approved
			return core.ResponseImpactUpdated
		},
	)

	_, err := wrapped.Call(t.Context(), `{"id":42}`)
	pe, ok := errors.AsType[*hitl.PauseError](err)
	if !ok {
		t.Fatalf("expected PauseError, got %v", err)
	}

	prompt := pe.Request.PromptAny()
	capturedMsg, _ = prompt.(string)
	if capturedMsg != `confirm delete: {"id":42}` {
		t.Fatalf("prompter not invoked with raw arguments, got %q", capturedMsg)
	}

	// Simulate the user replying yes.
	impact, err := pe.Request.OnResponseAny(true)
	if err != nil {
		t.Fatalf("OnResponseAny: %v", err)
	}
	if !captured {
		t.Fatal("onResponse handler not fired")
	}
	if impact != core.ResponseImpactUpdated {
		t.Fatalf("impact = %v, want Updated", impact)
	}
}

func TestRequireType_DeliversTypedValue(t *testing.T) {
	type Address struct {
		Street string
		City   string
	}

	inner := newFake("ship", "shipped")

	var got Address
	wrapped := hitl.RequireType[Address](
		inner,
		func(args string) string { return "need shipping address for: " + args },
		func(v Address) core.ResponseImpact {
			got = v
			return core.ResponseImpactUpdated
		},
	)

	_, err := wrapped.Call(t.Context(), `{"orderId":7}`)
	pe, ok := errors.AsType[*hitl.PauseError](err)
	if !ok {
		t.Fatalf("expected PauseError, got %v", err)
	}

	want := Address{Street: "1 Main St", City: "Springfield"}
	impact, err := pe.Request.OnResponseAny(want)
	if err != nil {
		t.Fatalf("OnResponseAny: %v", err)
	}
	if got != want {
		t.Fatalf("typed handler got %+v, want %+v", got, want)
	}
	if impact != core.ResponseImpactUpdated {
		t.Fatalf("impact = %v, want Updated", impact)
	}

	// Wrong-type response should error.
	_, err = pe.Request.OnResponseAny("not an address")
	if err == nil {
		t.Fatal("expected type error on mismatched response value")
	}
}

// fakeProcess is the minimum Process implementation HandlePause needs:
// AwaitInput is the only method invoked. Everything else panics so a
// regression that touches Process via HandlePause fails loudly.
type fakeProcess struct {
	core.Process // embed → unimplemented methods panic on call
	got          core.Awaitable
}

func (p *fakeProcess) AwaitInput(req core.Awaitable) core.ActionStatus {
	p.got = req
	return core.ActionWaiting
}

func TestHandlePause_RoutesPauseErrorToAwaitInput(t *testing.T) {
	fp := &fakeProcess{}
	pc := core.NewProcessContext(core.ProcessContextConfig{Process: fp})

	a := hitl.NewConfirmation("ok?", func(bool) core.ResponseImpact { return core.ResponseImpactUnchanged })
	pe := &hitl.PauseError{Request: a}

	status, paused := hitl.HandlePause(pc, pe)
	if !paused {
		t.Fatal("expected paused=true on PauseError")
	}
	if status != core.ActionWaiting {
		t.Fatalf("status = %v, want ActionWaiting", status)
	}
	if fp.got == nil || fp.got.ID() != a.ID() {
		t.Fatalf("AwaitInput got %v, want %v", fp.got, a)
	}
}

func TestHandlePause_PassesThroughNonPauseError(t *testing.T) {
	fp := &fakeProcess{}
	pc := core.NewProcessContext(core.ProcessContextConfig{Process: fp})

	plain := errors.New("ordinary failure")
	_, paused := hitl.HandlePause(pc, plain)
	if paused {
		t.Fatal("expected paused=false on non-PauseError")
	}
	if fp.got != nil {
		t.Fatal("AwaitInput must not run for non-PauseError")
	}
}

func TestPauseErrorMessageMentionsID(t *testing.T) {
	a := hitl.NewConfirmation("ok?", func(bool) core.ResponseImpact { return core.ResponseImpactUnchanged })
	pe := &hitl.PauseError{Request: a}

	msg := pe.Error()
	if !contains(msg, a.ID()) {
		t.Fatalf("PauseError.Error() = %q, expected to contain %q", msg, a.ID())
	}
}

func contains(haystack, needle string) bool {
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// Sanity: WithAwaiting should panic on nil tool / nil decider —
// programming errors should surface at boot.
func TestWithAwaiting_PanicsOnNilArgs(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil tool")
		}
	}()
	hitl.WithAwaiting(nil, func(context.Context, string) core.Awaitable { return nil })
}

func TestWithAwaiting_PanicsOnNilDecider(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil decider")
		}
	}()
	hitl.WithAwaiting(newFake("x", ""), nil)
}
