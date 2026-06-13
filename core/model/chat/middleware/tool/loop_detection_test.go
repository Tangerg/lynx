package tool

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

// --- unit: loopDetector.observe --------------------------------------------

// TestLoopDetector_TripsOnlyAboveThreshold pins the boundary: with the
// default window (10) and threshold (5), the same signature is tolerated
// up to and including the 5th occurrence and trips on the 6th — matching
// the field-proven Crush semantics (count > threshold).
func TestLoopDetector_TripsOnlyAboveThreshold(t *testing.T) {
	d := newLoopDetector(&LoopDetectionConfig{})
	if d.window != DefaultLoopWindow || d.threshold != DefaultLoopThreshold {
		t.Fatalf("defaults not applied: window=%d threshold=%d", d.window, d.threshold)
	}
	const sig = "abc"
	for i := 1; i <= DefaultLoopThreshold; i++ {
		if err := d.observe(sig); err != nil {
			t.Fatalf("occurrence %d tripped (%v); threshold is %d", i, err, d.threshold)
		}
	}
	var loopErr *LoopDetectedError
	if err := d.observe(sig); !errors.As(err, &loopErr) { // the 6th
		t.Fatalf("occurrence %d did not trip; want *LoopDetectedError, got %v", DefaultLoopThreshold+1, err)
	}
	if loopErr.Count != DefaultLoopThreshold+1 {
		t.Fatalf("Count = %d, want %d", loopErr.Count, DefaultLoopThreshold+1)
	}
}

// TestLoopDetector_DistinctSignaturesNeverTrip confirms a loop that keeps
// changing what it does is never flagged, regardless of length.
func TestLoopDetector_DistinctSignaturesNeverTrip(t *testing.T) {
	d := newLoopDetector(&LoopDetectionConfig{Window: 4, Threshold: 2})
	for i := range 20 {
		// A fresh signature every round (a..t, all distinct).
		if err := d.observe(string(rune('a' + i))); err != nil {
			t.Fatalf("distinct signatures tripped at %d: %v", i, err)
		}
	}
}

// TestLoopDetector_WindowEvicts verifies old occurrences age out: with a
// window of 3 and threshold 2, three identical sigs spaced by two others
// never accumulate to >2 inside any single window.
func TestLoopDetector_WindowEvicts(t *testing.T) {
	d := newLoopDetector(&LoopDetectionConfig{Window: 3, Threshold: 2})
	seq := []string{"x", "a", "b", "x", "c", "d", "x"} // x's are >2 apart
	for i, s := range seq {
		if err := d.observe(s); err != nil {
			t.Fatalf("evicting window tripped at %d: %v", i, err)
		}
	}
}

// TestLoopDetector_EmptySignatureIgnored — a round that ran no tools must
// not count toward the loop window.
func TestLoopDetector_EmptySignatureIgnored(t *testing.T) {
	d := newLoopDetector(&LoopDetectionConfig{Window: 2, Threshold: 1})
	for range 10 {
		if err := d.observe(""); err != nil {
			t.Fatalf("empty signature counted: %v", err)
		}
	}
}

func TestNewLoopDetector_NilDisables(t *testing.T) {
	if newLoopDetector(nil) != nil {
		t.Fatal("nil config should disable detection (nil detector)")
	}
}

// --- unit: roundSignature ---------------------------------------------------

func toolCall(id, name, args string) *chat.ToolCallPart {
	return &chat.ToolCallPart{ID: id, Name: name, Arguments: args}
}

func toolMsg(t *testing.T, returns ...*chat.ToolReturn) *chat.ToolMessage {
	t.Helper()
	tm, err := chat.NewToolMessage(returns)
	if err != nil {
		t.Fatalf("NewToolMessage: %v", err)
	}
	return tm
}

// TestRoundSignature_ResultSensitive is the crux of "don't flag a retry":
// identical calls with DIFFERENT results hash differently, so a tool whose
// output keeps changing never looks like a fixed point.
func TestRoundSignature_ResultSensitive(t *testing.T) {
	calls := []*chat.ToolCallPart{toolCall("1", "read", `{"p":"a"}`)}
	a := roundSignature(calls, toolMsg(t, &chat.ToolReturn{ID: "1", Name: "read", Result: "v1"}))
	b := roundSignature(calls, toolMsg(t, &chat.ToolReturn{ID: "1", Name: "read", Result: "v2"}))
	if a == b {
		t.Fatal("different results produced the same signature; retries would be misflagged")
	}
}

// TestRoundSignature_IgnoresCallID — same (name, args, result) but a fresh
// call ID each round must hash the same, or per-call IDs would defeat the
// detector (the Crush lesson).
func TestRoundSignature_IgnoresCallID(t *testing.T) {
	a := roundSignature(
		[]*chat.ToolCallPart{toolCall("id-1", "bash", `{"cmd":"ls"}`)},
		toolMsg(t, &chat.ToolReturn{ID: "id-1", Name: "bash", Result: "out"}),
	)
	b := roundSignature(
		[]*chat.ToolCallPart{toolCall("id-2", "bash", `{"cmd":"ls"}`)},
		toolMsg(t, &chat.ToolReturn{ID: "id-2", Name: "bash", Result: "out"}),
	)
	if a != b {
		t.Fatal("call ID leaked into the signature; differing IDs would defeat detection")
	}
}

func TestRoundSignature_NoToolsIsEmpty(t *testing.T) {
	if sig := roundSignature(nil, nil); sig != "" {
		t.Fatalf("no-tool round signature = %q, want empty", sig)
	}
}

// --- integration: through the middleware -----------------------------------

// TestToolMiddleware_LoopDetectionHalts verifies a genuinely stuck model
// (same tool + args, constant result) is halted with a LoopDetectedError on
// the 6th identical round — well before the iteration cap.
func TestToolMiddleware_LoopDetectionHalts(t *testing.T) {
	model := newFakeChatModel(t)
	calls := 0
	model.respond = func(*chat.Request) (*chat.Response, error) {
		calls++
		return responseWithToolCall(t, "echo", `{"x":1}`), nil
	}
	// Constant result regardless of args → identical round signature each time.
	echoTool := mustNewCallable(t, "echo", false, func(context.Context, string) (string, error) {
		return "same", nil
	})

	callMW, _ := NewMiddleware(Config{
		MaxIterations: 50, // high, so loop detection (not the cap) is what fires
		LoopDetection: &LoopDetectionConfig{},
	})
	req, _ := chat.NewClientRequest(model)
	req.WithMiddlewares(callMW).WithMessages(chat.NewUserMessage("seed")).WithTools(echoTool)

	_, err := req.Call().Response(context.Background())
	loopErr, ok := errors.AsType[*LoopDetectedError](err)
	if !ok {
		t.Fatalf("expected LoopDetectedError, got %v", err)
	}
	if loopErr.Count != DefaultLoopThreshold+1 {
		t.Fatalf("Count = %d, want %d", loopErr.Count, DefaultLoopThreshold+1)
	}
	if calls != DefaultLoopThreshold+1 {
		t.Fatalf("model invoked %d times, want %d (halt on the round that breaches the threshold)", calls, DefaultLoopThreshold+1)
	}
}

// TestToolMiddleware_LoopDetectionOptIn confirms the feature is off by
// default: the same stuck model runs to the iteration cap (no
// LoopDetectedError) when LoopDetection is unset.
func TestToolMiddleware_LoopDetectionOptIn(t *testing.T) {
	model := newFakeChatModel(t)
	model.respond = func(*chat.Request) (*chat.Response, error) {
		return responseWithToolCall(t, "echo", `{"x":1}`), nil
	}
	echoTool := mustNewCallable(t, "echo", false, func(context.Context, string) (string, error) {
		return "same", nil
	})

	callMW, _ := NewMiddleware(Config{MaxIterations: 3}) // no LoopDetection
	req, _ := chat.NewClientRequest(model)
	req.WithMiddlewares(callMW).WithMessages(chat.NewUserMessage("seed")).WithTools(echoTool)

	_, err := req.Call().Response(context.Background())
	if _, ok := errors.AsType[*LoopDetectedError](err); ok {
		t.Fatal("loop detection fired without being enabled")
	}
	if _, ok := errors.AsType[*MaxIterationsError](err); !ok {
		t.Fatalf("expected MaxIterationsError with detection off, got %v", err)
	}
}
