package chat

import (
	"testing"
)

func TestAccumulator_Empty(t *testing.T) {
	var acc Accumulator
	out := acc.Build()
	if len(out) != 0 {
		t.Errorf("empty accumulator should produce no parts; got %d", len(out))
	}
}

func TestAccumulator_AddNilIgnored(t *testing.T) {
	var acc Accumulator
	acc.Add(nil)
	out := acc.Build()
	if len(out) != 0 {
		t.Errorf("nil delta should be ignored; got %d parts", len(out))
	}
}

func TestAccumulator_SingleTextRun(t *testing.T) {
	var acc Accumulator
	acc.Add(&TextPart{Text: "Hel"})
	acc.Add(&TextPart{Text: "lo"})
	acc.Add(&TextPart{Text: " world"})
	out := acc.Build()
	if len(out) != 1 {
		t.Fatalf("3 same-type deltas should merge to 1 part; got %d", len(out))
	}
	tp, ok := out[0].(*TextPart)
	if !ok {
		t.Fatalf("expected TextPart; got %T", out[0])
	}
	if tp.Text != "Hello world" {
		t.Errorf("Text = %q", tp.Text)
	}
}

func TestAccumulator_TypeBoundaryFlushes(t *testing.T) {
	var acc Accumulator
	acc.Add(&TextPart{Text: "a"})
	acc.Add(&ReasoningPart{Text: "thinking"})
	acc.Add(&TextPart{Text: "b"})
	out := acc.Build()
	if len(out) != 3 {
		t.Fatalf("type changes should flush; got %d parts", len(out))
	}
	if out[0].Kind() != PartKindText || out[1].Kind() != PartKindReasoning || out[2].Kind() != PartKindText {
		t.Errorf("part order wrong: %s, %s, %s", out[0].Kind(), out[1].Kind(), out[2].Kind())
	}
	// Verify content separation: TextParts didn't get merged across the
	// reasoning boundary.
	if t0 := out[0].(*TextPart).Text; t0 != "a" {
		t.Errorf("first TextPart = %q; want %q", t0, "a")
	}
	if t2 := out[2].(*TextPart).Text; t2 != "b" {
		t.Errorf("third TextPart = %q; want %q", t2, "b")
	}
}

func TestAccumulator_InterleavedTextAndToolCalls(t *testing.T) {
	// Simulates Claude's text → tool_use → text → tool_use → text pattern.
	var acc Accumulator
	acc.Add(&TextPart{Text: "查天气："})
	acc.Add(&ToolCallPart{ID: "tu_1", Name: "weather", Arguments: "{\"city\":\"BJ\"}", State: ToolCallStateInputComplete})
	acc.Add(&TextPart{Text: "查日历："})
	acc.Add(&ToolCallPart{ID: "tu_2", Name: "calendar", Arguments: "{}", State: ToolCallStateInputComplete})
	acc.Add(&TextPart{Text: "等结果。"})
	out := acc.Build()

	if len(out) != 5 {
		t.Fatalf("expected 5 parts, got %d", len(out))
	}
	wantKinds := []PartKind{
		PartKindText, PartKindToolCall, PartKindText, PartKindToolCall, PartKindText,
	}
	for i, p := range out {
		if p.Kind() != wantKinds[i] {
			t.Errorf("part %d kind = %s; want %s", i, p.Kind(), wantKinds[i])
		}
	}
}

func TestAccumulator_ToolCallDifferentIDFlushes(t *testing.T) {
	var acc Accumulator
	acc.Add(&ToolCallPart{ID: "tc_1", Name: "a", Arguments: "{}", State: ToolCallStateInputComplete})
	acc.Add(&ToolCallPart{ID: "tc_2", Name: "b", Arguments: "{}", State: ToolCallStateInputComplete})
	out := acc.Build()
	if len(out) != 2 {
		t.Fatalf("different-ID tool calls should remain 2 parts; got %d", len(out))
	}
	if out[0].(*ToolCallPart).ID != "tc_1" || out[1].(*ToolCallPart).ID != "tc_2" {
		t.Errorf("IDs lost: %+v, %+v", out[0], out[1])
	}
}

func TestAccumulator_ToolCallSameIDArgsAccumulate(t *testing.T) {
	// OpenAI Chat Completions style: arguments arrive in fragments.
	var acc Accumulator
	acc.Add(&ToolCallPart{ID: "tc_1", Name: "weather", Arguments: "{\"c", State: ToolCallStateInputStreaming})
	acc.Add(&ToolCallPart{ID: "tc_1", Arguments: "ity\":"})
	acc.Add(&ToolCallPart{ID: "tc_1", Arguments: "\"BJ\"}", State: ToolCallStateInputComplete})
	out := acc.Build()
	if len(out) != 1 {
		t.Fatalf("same-ID tool call deltas should merge to 1; got %d", len(out))
	}
	tcp := out[0].(*ToolCallPart)
	if tcp.Arguments != "{\"city\":\"BJ\"}" {
		t.Errorf("Arguments = %q", tcp.Arguments)
	}
	if tcp.State != ToolCallStateInputComplete {
		t.Errorf("State = %s; want input_complete", tcp.State)
	}
}

func TestAccumulator_AddAll(t *testing.T) {
	var acc Accumulator
	acc.AddAll([]OutputPart{
		&TextPart{Text: "a"},
		&TextPart{Text: "b"},
		&ReasoningPart{Text: "r"},
	})
	out := acc.Build()
	if len(out) != 2 {
		t.Fatalf("expected 2 parts; got %d", len(out))
	}
	if out[0].(*TextPart).Text != "ab" {
		t.Errorf("first part text = %q", out[0].(*TextPart).Text)
	}
}

func TestAccumulator_BuildIdempotent(t *testing.T) {
	var acc Accumulator
	acc.Add(&TextPart{Text: "a"})
	acc.Add(&TextPart{Text: "b"})
	first := acc.Build()
	second := acc.Build()
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("idempotent Build failed: first=%d second=%d", len(first), len(second))
	}
	if &first[0] != &second[0] && first[0] != second[0] {
		t.Error("second Build should return the same parts")
	}
}

func TestAccumulator_Reset(t *testing.T) {
	var acc Accumulator
	acc.Add(&TextPart{Text: "a"})
	acc.Reset()
	out := acc.Build()
	if len(out) != 0 {
		t.Errorf("after Reset, Build should produce 0 parts; got %d", len(out))
	}
	// Reusable
	acc.Add(&TextPart{Text: "x"})
	out = acc.Build()
	if len(out) != 1 || out[0].(*TextPart).Text != "x" {
		t.Errorf("accumulator not reusable after Reset")
	}
}

// TestAccumulator_TypeAgnostic confirms the accumulator does not branch
// on concrete part types — adding a new (hypothetical) part type that
// returns false from appendDelta would still produce one part per
// instance without code changes here.
//
// We simulate the "add a new part type" scenario by interleaving three
// kinds and asserting flush behavior is purely based on the
// appendDelta contract.
func TestAccumulator_TypeAgnosticFlushSemantics(t *testing.T) {
	var acc Accumulator
	// Mix of all three kinds; same-type runs should merge, type changes flush.
	acc.Add(&TextPart{Text: "1"})
	acc.Add(&TextPart{Text: "2"})
	acc.Add(&ReasoningPart{Text: "r1"})
	acc.Add(&ReasoningPart{Text: "r2"})
	acc.Add(&ToolCallPart{ID: "tc", Arguments: "{"})
	acc.Add(&ToolCallPart{ID: "tc", Arguments: "}"})
	acc.Add(&TextPart{Text: "3"})
	out := acc.Build()

	if len(out) != 4 {
		t.Fatalf("expected 4 parts after merge; got %d", len(out))
	}
	if out[0].(*TextPart).Text != "12" {
		t.Errorf("first text merge failed: %q", out[0].(*TextPart).Text)
	}
	if out[1].(*ReasoningPart).Text != "r1r2" {
		t.Errorf("reasoning merge failed: %q", out[1].(*ReasoningPart).Text)
	}
	if out[2].(*ToolCallPart).Arguments != "{}" {
		t.Errorf("toolcall merge failed: %q", out[2].(*ToolCallPart).Arguments)
	}
	if out[3].(*TextPart).Text != "3" {
		t.Errorf("trailing text wrong: %q", out[3].(*TextPart).Text)
	}
}
