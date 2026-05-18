package chat

import (
	"bytes"
	"testing"
)

func TestTextPart_AppendDelta_SameType(t *testing.T) {
	p := &TextPart{Text: "Hello"}
	ok := p.appendDelta(&TextPart{Text: " world"})
	if !ok {
		t.Fatal("appendDelta of same type should merge")
	}
	if p.Text != "Hello world" {
		t.Errorf("Text = %q; want %q", p.Text, "Hello world")
	}
}

func TestTextPart_AppendDelta_DifferentType(t *testing.T) {
	p := &TextPart{Text: "Hello"}
	if ok := p.appendDelta(&ReasoningPart{Text: "..."}); ok {
		t.Fatal("appendDelta of different type should NOT merge")
	}
	if p.Text != "Hello" {
		t.Errorf("Text changed after rejected delta: got %q", p.Text)
	}
}

func TestReasoningPart_AppendDelta_ConcatenatesAndKeepsSig(t *testing.T) {
	p := &ReasoningPart{Text: "think ", Signature: []byte("sig1")}
	ok := p.appendDelta(&ReasoningPart{Text: "more"})
	if !ok {
		t.Fatal("same-type reasoning delta should merge")
	}
	if p.Text != "think more" {
		t.Errorf("Text = %q", p.Text)
	}
	// Empty incoming signature should preserve existing.
	if !bytes.Equal(p.Signature, []byte("sig1")) {
		t.Errorf("Signature lost: %s", p.Signature)
	}
}

func TestReasoningPart_AppendDelta_SignatureOverwritten(t *testing.T) {
	p := &ReasoningPart{Text: "a", Signature: []byte("old")}
	p.appendDelta(&ReasoningPart{Text: "b", Signature: []byte("new")})
	if !bytes.Equal(p.Signature, []byte("new")) {
		t.Errorf("Signature not overwritten: %s", p.Signature)
	}
}

func TestReasoningPart_AppendDelta_RejectsOtherType(t *testing.T) {
	p := &ReasoningPart{Text: "a"}
	if ok := p.appendDelta(&TextPart{Text: "b"}); ok {
		t.Fatal("reasoning should not merge with text")
	}
}

func TestToolCallPart_AppendDelta_SameID(t *testing.T) {
	p := &ToolCallPart{
		ID:        "tc_1",
		Name:      "weather",
		Arguments: "{\"c",
	}
	ok := p.appendDelta(&ToolCallPart{
		ID:        "tc_1",
		Arguments: "ity\":\"BJ\"}",
	})
	if !ok {
		t.Fatal("same-ID delta should merge")
	}
	if p.Arguments != "{\"city\":\"BJ\"}" {
		t.Errorf("Arguments = %q", p.Arguments)
	}
}

func TestToolCallPart_AppendDelta_EmptyIDContinues(t *testing.T) {
	p := &ToolCallPart{ID: "tc_1", Arguments: "{"}
	ok := p.appendDelta(&ToolCallPart{ID: "", Arguments: "}"})
	if !ok {
		t.Fatal("empty-ID delta should continue in-flight call")
	}
	if p.Arguments != "{}" {
		t.Errorf("Arguments = %q", p.Arguments)
	}
}

func TestToolCallPart_AppendDelta_DifferentIDRejected(t *testing.T) {
	p := &ToolCallPart{ID: "tc_1", Arguments: "{}"}
	if ok := p.appendDelta(&ToolCallPart{ID: "tc_2", Arguments: "..."}); ok {
		t.Fatal("different-ID delta must NOT merge")
	}
	if p.Arguments != "{}" {
		t.Error("rejected delta should not mutate target")
	}
}

func TestToolCallPart_AppendDelta_NameSetOnce(t *testing.T) {
	// Original missing name; delta provides it.
	p := &ToolCallPart{ID: "tc_1"}
	p.appendDelta(&ToolCallPart{ID: "tc_1", Name: "weather"})
	if p.Name != "weather" {
		t.Errorf("Name = %q; want weather", p.Name)
	}

	// Subsequent delta with a different name should not overwrite —
	// vendors emit the canonical name in the first chunk only.
	p.appendDelta(&ToolCallPart{ID: "tc_1", Name: "ignored"})
	if p.Name != "weather" {
		t.Errorf("Name overwritten: %q", p.Name)
	}
}

func TestToolCallPart_AppendDelta_RejectsOtherType(t *testing.T) {
	p := &ToolCallPart{ID: "tc_1"}
	if ok := p.appendDelta(&TextPart{Text: "x"}); ok {
		t.Fatal("toolcall should not merge with text")
	}
}

func TestPartKind_AllParts(t *testing.T) {
	cases := []struct {
		part OutputPart
		want PartKind
	}{
		{&TextPart{}, PartKindText},
		{&ReasoningPart{}, PartKindReasoning},
		{&ToolCallPart{}, PartKindToolCall},
	}
	for _, c := range cases {
		if got := c.part.Kind(); got != c.want {
			t.Errorf("%T Kind() = %s; want %s", c.part, got, c.want)
		}
	}
}

// Compile-time sealed-union check: only types in this package can
// satisfy OutputPart because appendDelta is unexported.
var (
	_ OutputPart = (*TextPart)(nil)
	_ OutputPart = (*ReasoningPart)(nil)
	_ OutputPart = (*ToolCallPart)(nil)
)
