package chat

import "testing"

func TestIsThoughtMessage(t *testing.T) {
	if IsThoughtMessage(nil) {
		t.Fatalf("nil should not be a thought message")
	}
	if IsThoughtMessage(NewAssistantMessage("hi")) {
		t.Fatalf("plain text message should not be a thought message")
	}

	thinking := NewAssistantMessage(MessageParams{
		Text: "step 1...",
		Metadata: map[string]any{
			MetaIsThought:         true,
			MetaThinkingSignature: "sig-abc",
		},
	})
	if !IsThoughtMessage(thinking) {
		t.Fatalf("thinking message should report IsThoughtMessage=true")
	}
	if got := ThinkingSignature(thinking); got != "sig-abc" {
		t.Fatalf("ThinkingSignature: want sig-abc, got %q", got)
	}
}

func TestRedactedThinkingData(t *testing.T) {
	redacted := NewAssistantMessage(map[string]any{
		MetaIsThought:            true,
		MetaRedactedThinkingData: "opaque-blob",
	})
	if !IsThoughtMessage(redacted) {
		t.Fatalf("redacted thinking should still be flagged as thought")
	}
	if got := RedactedThinkingData(redacted); got != "opaque-blob" {
		t.Fatalf("RedactedThinkingData: want opaque-blob, got %q", got)
	}
}

func TestReasoningContent(t *testing.T) {
	msg := NewAssistantMessage(map[string]any{
		MetaReasoningContent: "because…",
	})
	if got := ReasoningContent(msg); got != "because…" {
		t.Fatalf("ReasoningContent: got %q", got)
	}
	if ReasoningContent(nil) != "" {
		t.Fatalf("ReasoningContent(nil) must be empty string")
	}
}

func TestResponse_ThoughtsAndOutputText_MultiResultPattern(t *testing.T) {
	thinking := NewAssistantMessage(MessageParams{
		Text: "let me think...",
		Metadata: map[string]any{
			MetaIsThought:         true,
			MetaThinkingSignature: "sig",
		},
	})
	main := NewAssistantMessage("the answer is 42")

	resp := &Response{
		Results: []*Result{
			{AssistantMessage: thinking, Metadata: &ResultMetadata{}},
			{AssistantMessage: main, Metadata: &ResultMetadata{}},
		},
		Metadata: &ResponseMetadata{},
	}

	if got := resp.Thoughts(); got != "let me think..." {
		t.Fatalf("Thoughts (multi-result): want %q, got %q", "let me think...", got)
	}
	if got := resp.OutputText(); got != "the answer is 42" {
		t.Fatalf("OutputText (multi-result): want %q, got %q", "the answer is 42", got)
	}
}

func TestResponse_ThoughtsAndOutputText_MetadataChannelPattern(t *testing.T) {
	main := NewAssistantMessage(MessageParams{
		Text: "Paris.",
		Metadata: map[string]any{
			MetaReasoningContent: "France's capital is well known…",
		},
	})

	resp := &Response{
		Results:  []*Result{{AssistantMessage: main, Metadata: &ResultMetadata{}}},
		Metadata: &ResponseMetadata{},
	}

	if got := resp.Thoughts(); got != "France's capital is well known…" {
		t.Fatalf("Thoughts (metadata-channel): got %q", got)
	}
	if got := resp.OutputText(); got != "Paris." {
		t.Fatalf("OutputText (metadata-channel): got %q", got)
	}
}

func TestResponseAccumulator_ConcatenatesReasoningContent(t *testing.T) {
	acc := NewResponseAccumulator()

	chunk1 := &Response{
		Results: []*Result{{
			AssistantMessage: NewAssistantMessage(MessageParams{
				Text: "Pa",
				Metadata: map[string]any{
					MetaReasoningContent: "First, ",
				},
			}),
			Metadata: &ResultMetadata{},
		}},
		Metadata: &ResponseMetadata{},
	}
	chunk2 := &Response{
		Results: []*Result{{
			AssistantMessage: NewAssistantMessage(MessageParams{
				Text: "ris.",
				Metadata: map[string]any{
					MetaReasoningContent: "France is in Europe.",
				},
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
	if got := ReasoningContent(final); got != "First, France is in Europe." {
		t.Fatalf("reasoning concat: want %q, got %q", "First, France is in Europe.", got)
	}
}
