package chat

import (
	"testing"
)

func TestPartAccumulator_Empty(t *testing.T) {
	var acc partAccumulator
	out := acc.build()
	if len(out) != 0 {
		t.Errorf("empty accumulator should produce no parts; got %d", len(out))
	}
}

func TestPartAccumulator_AddNilIgnored(t *testing.T) {
	var acc partAccumulator
	acc.add(nil)
	out := acc.build()
	if len(out) != 0 {
		t.Errorf("nil delta should be ignored; got %d parts", len(out))
	}
}

func TestPartAccumulator_SingleTextRun(t *testing.T) {
	var acc partAccumulator
	acc.add(&TextPart{Text: "Hel"})
	acc.add(&TextPart{Text: "lo"})
	acc.add(&TextPart{Text: " world"})
	out := acc.build()
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

func TestPartAccumulator_TypeBoundaryFlushes(t *testing.T) {
	var acc partAccumulator
	acc.add(&TextPart{Text: "a"})
	acc.add(&ReasoningPart{Text: "thinking"})
	acc.add(&TextPart{Text: "b"})
	out := acc.build()
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

func TestPartAccumulator_InterleavedTextAndToolCalls(t *testing.T) {
	// Simulates Claude's text → tool_use → text → tool_use → text pattern.
	var acc partAccumulator
	acc.add(&TextPart{Text: "查天气："})
	acc.add(&ToolCallPart{ID: "tu_1", Name: "weather", Arguments: "{\"city\":\"BJ\"}"})
	acc.add(&TextPart{Text: "查日历："})
	acc.add(&ToolCallPart{ID: "tu_2", Name: "calendar", Arguments: "{}"})
	acc.add(&TextPart{Text: "等结果。"})
	out := acc.build()

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

func TestPartAccumulator_ToolCallDifferentIDFlushes(t *testing.T) {
	var acc partAccumulator
	acc.add(&ToolCallPart{ID: "tc_1", Name: "a", Arguments: "{}"})
	acc.add(&ToolCallPart{ID: "tc_2", Name: "b", Arguments: "{}"})
	out := acc.build()
	if len(out) != 2 {
		t.Fatalf("different-ID tool calls should remain 2 parts; got %d", len(out))
	}
	if out[0].(*ToolCallPart).ID != "tc_1" || out[1].(*ToolCallPart).ID != "tc_2" {
		t.Errorf("IDs lost: %+v, %+v", out[0], out[1])
	}
}

func TestPartAccumulator_ToolCallSameIDArgsAccumulate(t *testing.T) {
	// OpenAI Chat Completions style: arguments arrive in fragments.
	var acc partAccumulator
	acc.add(&ToolCallPart{ID: "tc_1", Name: "weather", Arguments: "{\"c"})
	acc.add(&ToolCallPart{ID: "tc_1", Arguments: "ity\":"})
	acc.add(&ToolCallPart{ID: "tc_1", Arguments: "\"BJ\"}"})
	out := acc.build()
	if len(out) != 1 {
		t.Fatalf("same-ID tool call deltas should merge to 1; got %d", len(out))
	}
	tcp := out[0].(*ToolCallPart)
	if tcp.Arguments != "{\"city\":\"BJ\"}" {
		t.Errorf("Arguments = %q", tcp.Arguments)
	}
}

func TestPartAccumulator_AddAll(t *testing.T) {
	var acc partAccumulator
	acc.addAll([]OutputPart{
		&TextPart{Text: "a"},
		&TextPart{Text: "b"},
		&ReasoningPart{Text: "r"},
	})
	out := acc.build()
	if len(out) != 2 {
		t.Fatalf("expected 2 parts; got %d", len(out))
	}
	if out[0].(*TextPart).Text != "ab" {
		t.Errorf("first part text = %q", out[0].(*TextPart).Text)
	}
}

func TestPartAccumulator_BuildIdempotent(t *testing.T) {
	var acc partAccumulator
	acc.add(&TextPart{Text: "a"})
	acc.add(&TextPart{Text: "b"})
	first := acc.build()
	second := acc.build()
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("idempotent build failed: first=%d second=%d", len(first), len(second))
	}
	if &first[0] != &second[0] && first[0] != second[0] {
		t.Error("second build should return the same parts")
	}
}

// TestPartAccumulator_TypeAgnostic confirms the accumulator does not branch
// on concrete part types — adding a new (hypothetical) part type that
// returns false from appendDelta would still produce one part per
// instance without code changes here.
//
// We simulate the "add a new part type" scenario by interleaving three
// kinds and asserting flush behavior is purely based on the
// appendDelta contract.
func TestPartAccumulator_TypeAgnosticFlushSemantics(t *testing.T) {
	var acc partAccumulator
	// Mix of all three kinds; same-type runs should merge, type changes flush.
	acc.add(&TextPart{Text: "1"})
	acc.add(&TextPart{Text: "2"})
	acc.add(&ReasoningPart{Text: "r1"})
	acc.add(&ReasoningPart{Text: "r2"})
	acc.add(&ToolCallPart{ID: "tc", Arguments: "{"})
	acc.add(&ToolCallPart{ID: "tc", Arguments: "}"})
	acc.add(&TextPart{Text: "3"})
	out := acc.build()

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
