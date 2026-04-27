package chat

import "testing"

func TestAssistantMessage_HasReasoning(t *testing.T) {
	cases := []struct {
		name string
		msg  *AssistantMessage
		want bool
	}{
		{"nil receiver", nil, false},
		{"empty message", &AssistantMessage{}, false},
		{"text only", &AssistantMessage{Text: "hi"}, false},
		{"reasoning set", &AssistantMessage{Reasoning: "let me think..."}, true},
		{"both set", &AssistantMessage{Text: "Paris.", Reasoning: "France's capital"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.msg.HasReasoning(); got != tc.want {
				t.Fatalf("HasReasoning: want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestNewAssistantMessage_WithReasoning(t *testing.T) {
	m := NewAssistantMessage(MessageParams{
		Text:      "Paris.",
		Reasoning: "France's capital is Paris.",
		Metadata:  map[string]any{"foo": "bar"},
	})
	if m.Text != "Paris." {
		t.Fatalf("Text: %q", m.Text)
	}
	if m.Reasoning != "France's capital is Paris." {
		t.Fatalf("Reasoning: %q", m.Reasoning)
	}
	if m.Metadata["foo"] != "bar" {
		t.Fatalf("Metadata not preserved")
	}
}

func TestResponse_Reasoning(t *testing.T) {
	resp := &Response{
		Results: []*Result{
			{AssistantMessage: NewAssistantMessage(MessageParams{
				Text:      "First answer",
				Reasoning: "First thought.",
			}), Metadata: &ResultMetadata{}},
			{AssistantMessage: NewAssistantMessage(MessageParams{
				Text:      "Second answer",
				Reasoning: " Second thought.",
			}), Metadata: &ResultMetadata{}},
		},
		Metadata: &ResponseMetadata{},
	}

	if got := resp.Reasoning(); got != "First thought. Second thought." {
		t.Fatalf("Reasoning: got %q", got)
	}
	if got := resp.OutputText(); got != "First answerSecond answer" {
		t.Fatalf("OutputText: got %q", got)
	}
}

func TestResponse_Reasoning_NoReasoning(t *testing.T) {
	resp := &Response{
		Results: []*Result{
			{AssistantMessage: NewAssistantMessage("plain answer"), Metadata: &ResultMetadata{}},
		},
		Metadata: &ResponseMetadata{},
	}
	if got := resp.Reasoning(); got != "" {
		t.Fatalf("expected empty reasoning, got %q", got)
	}
	if got := resp.OutputText(); got != "plain answer" {
		t.Fatalf("OutputText: got %q", got)
	}
}

func TestResponseAccumulator_ReasoningConcat(t *testing.T) {
	acc := NewResponseAccumulator()

	chunk1 := &Response{
		Results: []*Result{{
			AssistantMessage: NewAssistantMessage(MessageParams{
				Text:      "Pa",
				Reasoning: "First, ",
			}),
			Metadata: &ResultMetadata{},
		}},
		Metadata: &ResponseMetadata{},
	}
	chunk2 := &Response{
		Results: []*Result{{
			AssistantMessage: NewAssistantMessage(MessageParams{
				Text:      "ris.",
				Reasoning: "France is in Europe.",
			}),
			Metadata: &ResultMetadata{},
		}},
		Metadata: &ResponseMetadata{},
	}

	acc.AddChunk(chunk1)
	acc.AddChunk(chunk2)

	final := acc.Result().AssistantMessage
	if final.Text != "Paris." {
		t.Fatalf("text concat: want %q, got %q", "Paris.", final.Text)
	}
	if final.Reasoning != "First, France is in Europe." {
		t.Fatalf("reasoning concat: want %q, got %q", "First, France is in Europe.", final.Reasoning)
	}
}

func TestUsage_HasReasoningTokens(t *testing.T) {
	var u *Usage
	if u.HasReasoningTokens() {
		t.Fatalf("nil receiver should report false")
	}

	u = &Usage{PromptTokens: 10, CompletionTokens: 50}
	if u.HasReasoningTokens() {
		t.Fatalf("usage without reasoning tokens should report false")
	}
	if total := u.TotalTokens(); total != 60 {
		t.Fatalf("TotalTokens: want 60, got %d", total)
	}

	rt := int64(20)
	u.ReasoningTokens = &rt
	if !u.HasReasoningTokens() {
		t.Fatalf("usage with reasoning tokens should report true")
	}
	if total := u.TotalTokens(); total != 60 {
		t.Fatalf("TotalTokens must NOT include reasoning tokens (subset of completion); want 60 got %d", total)
	}
}
