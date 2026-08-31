package chat_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
	"github.com/Tangerg/scope/core/metadata"
)

func TestResponseAccumulatorPromotesTerminalStream(t *testing.T) {
	chunks := []*chat.ResponseDelta{
		{
			Parts:    []chat.PartDelta{chat.NewReasoningDelta("step ", []byte("state-"))},
			Metadata: &chat.ResponseMetadata{ID: "response-1", Model: "model-initial"},
		},
		{Parts: []chat.PartDelta{
			chat.NewReasoningDelta("one", []byte("final")),
			chat.NewTextDelta("hel"),
			chat.NewToolCallDelta(chat.ToolCallDelta{ID: "call-1", Name: "search", Arguments: `{"q":"`}),
		}},
		{
			Parts: []chat.PartDelta{
				chat.NewToolCallDelta(chat.ToolCallDelta{ID: "call-1", Name: "search", Arguments: `scope"}`}),
				chat.NewTextDelta("lo"),
			},
			FinishReason:   chat.FinishReasonToolCalls,
			OutputMetadata: &chat.OutputMetadata{},
			Metadata:       &chat.ResponseMetadata{Model: "model-final", Usage: chat.Usage{InputTokens: 12, OutputTokens: 5}},
		},
	}
	chunks[0].Metadata.Extra = metadata.Map{}
	if err := chunks[0].Metadata.Extra.Set("test/value", "first"); err != nil {
		t.Fatal(err)
	}
	if err := chunks[2].Metadata.Extra.Set("test/value", "last"); err != nil {
		t.Fatal(err)
	}
	if err := chunks[2].OutputMetadata.Extra.Set("test/finish", true); err != nil {
		t.Fatal(err)
	}

	var accumulator chat.ResponseAccumulator
	for _, chunk := range chunks {
		if err := accumulator.Add(chunk); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	response, err := accumulator.Response()
	if err != nil {
		t.Fatalf("Response: %v", err)
	}
	if response.Metadata.ID != "response-1" || response.Metadata.Model != "model-final" {
		t.Errorf("identity = %q/%q", response.Metadata.ID, response.Metadata.Model)
	}
	wantKinds := []chat.PartKind{chat.PartReasoning, chat.PartText, chat.PartToolCall, chat.PartText}
	if got := partKinds(response.Output.Message); !slices.Equal(got, wantKinds) {
		t.Fatalf("part kinds = %v; want %v", got, wantKinds)
	}
	if response.Output.Message.Parts[0].Text != "step one" || string(response.Output.Message.Parts[0].ReasoningState) != "state-final" {
		t.Errorf("reasoning = %#v", response.Output.Message.Parts[0])
	}
	call := response.Output.Message.Parts[2].ToolCall
	if call == nil || call.ID != "call-1" || call.Name != "search" || call.Arguments != `{"q":"scope"}` {
		t.Errorf("tool call = %#v", call)
	}
	if response.Output.FinishReason != chat.FinishReasonToolCalls {
		t.Errorf("finish = %q", response.Output.FinishReason)
	}
	if response.Metadata.Usage.InputTokens != 12 || response.Metadata.Usage.OutputTokens != 5 {
		t.Errorf("usage = %#v", response.Metadata.Usage)
	}
	if got := decode[string](t, response.Metadata.Extra, "test/value"); got != "last" {
		t.Errorf("response metadata = %q", got)
	}
	if got := decode[bool](t, response.Output.Metadata.Extra, "test/finish"); !got {
		t.Error("output metadata was not merged")
	}
}

func TestResponseAccumulatorMergesInterleavedParallelToolCalls(t *testing.T) {
	chunks := []*chat.ResponseDelta{
		{Parts: []chat.PartDelta{
			chat.NewToolCallDelta(chat.ToolCallDelta{ID: "call-a", Name: "a", Arguments: `{"a":`}),
			chat.NewToolCallDelta(chat.ToolCallDelta{ID: "call-b", Name: "b", Arguments: `{"b":`}),
		}},
		{Parts: []chat.PartDelta{
			chat.NewToolCallDelta(chat.ToolCallDelta{ID: "call-a", Name: "a", Arguments: `1}`}),
			chat.NewToolCallDelta(chat.ToolCallDelta{ID: "call-b", Name: "b", Arguments: `2}`}),
		}, FinishReason: chat.FinishReasonToolCalls},
	}
	var accumulator chat.ResponseAccumulator
	for _, chunk := range chunks {
		if err := accumulator.Add(chunk); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	response, err := accumulator.Response()
	if err != nil {
		t.Fatal(err)
	}
	parts := response.Output.Message.Parts
	if len(parts) != 2 || parts[0].ToolCall.Arguments != `{"a":1}` || parts[1].ToolCall.Arguments != `{"b":2}` {
		t.Fatalf("parallel tools = %#v", parts)
	}
}

func TestResponseAccumulatorAttachesCitationsAndRefusal(t *testing.T) {
	citation := chat.Citation{Source: chat.CitationSource{Kind: chat.CitationSourceURI, Value: "https://example.com/source"}, Title: "Source"}
	chunks := []*chat.ResponseDelta{
		{Parts: []chat.PartDelta{chat.NewTextDelta("claim"), chat.NewCitationDelta(citation)}},
		{Parts: []chat.PartDelta{chat.NewRefusalDelta("cannot continue")}, FinishReason: chat.FinishReasonRefusal},
	}
	var accumulator chat.ResponseAccumulator
	for _, chunk := range chunks {
		if err := accumulator.Add(chunk); err != nil {
			t.Fatal(err)
		}
	}
	response, err := accumulator.Response()
	if err != nil {
		t.Fatal(err)
	}
	parts := response.Output.Message.Parts
	if len(parts) != 2 || len(parts[0].Citations) != 1 || parts[1].Kind != chat.PartRefusal {
		t.Fatalf("parts = %#v", parts)
	}
}

func TestResponseAccumulatorPromotesCompleteMediaDelta(t *testing.T) {
	image, err := media.NewBytes("image/png", []byte("image"))
	if err != nil {
		t.Fatal(err)
	}
	var accumulator chat.ResponseAccumulator
	err = accumulator.Add(&chat.ResponseDelta{
		Parts:        []chat.PartDelta{chat.NewMediaDelta(image)},
		FinishReason: chat.FinishReasonStop,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := accumulator.Response()
	if err != nil {
		t.Fatal(err)
	}
	part := response.Output.Message.Parts[0]
	if part.Kind != chat.PartMedia || part.Media == image {
		t.Fatalf("media part = %#v", part)
	}
}

func TestResponseAccumulatorUsesLatestUsageSnapshot(t *testing.T) {
	reasoning := int64(2)
	chunks := []*chat.ResponseDelta{
		{Metadata: &chat.ResponseMetadata{Usage: chat.Usage{InputTokens: 8}}},
		{Metadata: &chat.ResponseMetadata{Usage: chat.Usage{InputTokens: 8, OutputTokens: 3, ReasoningTokens: &reasoning}}, FinishReason: chat.FinishReasonStop},
	}
	var accumulator chat.ResponseAccumulator
	for _, chunk := range chunks {
		if err := accumulator.Add(chunk); err != nil {
			t.Fatal(err)
		}
	}
	response, err := accumulator.Response()
	if err != nil {
		t.Fatal(err)
	}
	usage := response.Metadata.Usage
	if usage.InputTokens != 8 || usage.OutputTokens != 3 || usage.ReasoningTokens == nil || *usage.ReasoningTokens != 2 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestResponseAccumulatorDoesNotManufacturePartialResponse(t *testing.T) {
	var accumulator chat.ResponseAccumulator
	if err := accumulator.Add(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("partial")}}); err != nil {
		t.Fatal(err)
	}
	if accumulator.Text() != "partial" {
		t.Fatalf("Text = %q", accumulator.Text())
	}
	if _, err := accumulator.Response(); !errors.Is(err, chat.ErrInvalidResponse) {
		t.Fatalf("Response error = %v, want ErrInvalidResponse", err)
	}
	if err := accumulator.Add(&chat.ResponseDelta{FinishReason: chat.FinishReasonStop}); err != nil {
		t.Fatal(err)
	}
	first, err := accumulator.Response()
	if err != nil {
		t.Fatal(err)
	}
	first.Output.Message.Parts[0].Text = "mutated"
	second, err := accumulator.Response()
	if err != nil {
		t.Fatal(err)
	}
	if second.Text() != "partial" {
		t.Fatalf("snapshot mutation changed accumulator to %q", second.Text())
	}
}

func TestResponseAccumulatorRejectsConflictingToolIdentityAtomically(t *testing.T) {
	var accumulator chat.ResponseAccumulator
	if err := accumulator.Add(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewToolCallDelta(chat.ToolCallDelta{ID: "call-1", Name: "search", Arguments: "{"})}}); err != nil {
		t.Fatal(err)
	}
	err := accumulator.Add(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewToolCallDelta(chat.ToolCallDelta{ID: "call-1", Name: "lookup", Arguments: "}"})}})
	if err == nil {
		t.Fatal("Add accepted conflicting tool name")
	}
	addErr := accumulator.Add(&chat.ResponseDelta{
		Parts:        []chat.PartDelta{chat.NewToolCallDelta(chat.ToolCallDelta{ID: "call-1", Name: "search", Arguments: "}"})},
		FinishReason: chat.FinishReasonToolCalls,
	})
	if addErr != nil {
		t.Fatal(addErr)
	}
	response, err := accumulator.Response()
	if err != nil {
		t.Fatal(err)
	}
	call := response.Output.Message.Parts[0].ToolCall
	if call.Name != "search" || call.Arguments != "{}" {
		t.Fatalf("failed Add mutated accumulator: %#v", call)
	}
}

func TestResponseAccumulatorRejectsNilEmptyAndPostTerminalDeltas(t *testing.T) {
	var accumulator chat.ResponseAccumulator
	if _, err := accumulator.Response(); !errors.Is(err, chat.ErrInvalidResponse) {
		t.Fatalf("empty Response error = %v", err)
	}
	if err := accumulator.Add(nil); !errors.Is(err, chat.ErrInvalidResponse) {
		t.Fatalf("Add(nil) error = %v", err)
	}
	if err := accumulator.Add(&chat.ResponseDelta{}); !errors.Is(err, chat.ErrInvalidResponse) {
		t.Fatalf("Add(empty) error = %v", err)
	}
	if err := accumulator.Add(&chat.ResponseDelta{FinishReason: chat.FinishReasonStop}); err != nil {
		t.Fatal(err)
	}
	if err := accumulator.Add(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("late")}}); !errors.Is(err, chat.ErrInvalidResponse) {
		t.Fatalf("Add(post-terminal) error = %v, want ErrInvalidResponse", err)
	}
	var nilAccumulator *chat.ResponseAccumulator
	if err := nilAccumulator.Add(&chat.ResponseDelta{FinishReason: chat.FinishReasonStop}); err == nil {
		t.Fatal("nil accumulator accepted Add")
	}
}

func partKinds(message *chat.Message) []chat.PartKind {
	if message == nil {
		return nil
	}
	kinds := make([]chat.PartKind, len(message.Parts))
	for index := range message.Parts {
		kinds[index] = message.Parts[index].Kind
	}
	return kinds
}

func decode[T any](t *testing.T, values metadata.Map, key string) T {
	t.Helper()
	value, found, err := values.Decode[T](key)
	if err != nil {
		t.Fatalf("Decode %q: %v", key, err)
	}
	if !found {
		t.Fatalf("metadata %q not found", key)
	}
	return value
}
