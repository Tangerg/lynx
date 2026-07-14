package chat_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
)

func TestResponseAccumulatorAggregatesChoicesAndDeltas(t *testing.T) {
	chunks := []*chat.Response{
		{
			ID:    "response-1",
			Model: "model-initial",
			Choices: []chat.Choice{
				{
					Index:   0,
					Message: assistant(chat.NewReasoningPart("step ", []byte("sig-"))),
				},
				{
					Index:   1,
					Message: assistant(chat.NewTextPart("alter")),
				},
			},
		},
		{
			Choices: []chat.Choice{
				{
					Index:   1,
					Message: assistant(chat.NewTextPart("native")),
				},
				{
					Index: 0,
					Message: assistant(
						chat.NewReasoningPart("one", []byte("nature")),
						chat.NewTextPart("hel"),
						chat.NewToolCallPart(chat.ToolCall{ID: "call-1", Name: "search", Arguments: `{"q":"`}),
					),
				},
			},
		},
		{
			Model: "model-final",
			Choices: []chat.Choice{{
				Index: 0,
				Message: assistant(
					chat.NewToolCallPart(chat.ToolCall{ID: "call-1", Name: "search", Arguments: `lynx"}`}),
					chat.NewTextPart("lo"),
				),
				FinishReason: chat.FinishReasonToolCalls,
			}},
			Usage: chat.Usage{InputTokens: 12, OutputTokens: 5},
		},
	}
	if err := chunks[0].SetExtension("test/value", "first"); err != nil {
		t.Fatal(err)
	}
	if err := chunks[2].SetExtension("test/value", "last"); err != nil {
		t.Fatal(err)
	}
	if err := chunks[2].Choices[0].SetExtension("test/finish", true); err != nil {
		t.Fatal(err)
	}

	var accumulator chat.ResponseAccumulator
	for _, chunk := range chunks {
		if err := accumulator.Add(chunk); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	response := accumulator.Response()
	if response == nil {
		t.Fatal("Response returned nil")
	}
	if response.ID != "response-1" || response.Model != "model-final" {
		t.Errorf("identity = %q/%q", response.ID, response.Model)
	}
	if len(response.Choices) != 2 || response.Choices[0].Index != 0 || response.Choices[1].Index != 1 {
		t.Fatalf("choice order = %#v", response.Choices)
	}
	first := response.Choices[0]
	wantKinds := []chat.PartKind{chat.PartReasoning, chat.PartText, chat.PartToolCall, chat.PartText}
	if got := partKinds(first.Message); !slices.Equal(got, wantKinds) {
		t.Fatalf("part kinds = %v; want %v", got, wantKinds)
	}
	if first.Message.Parts[0].Text != "step one" || string(first.Message.Parts[0].Signature) != "sig-nature" {
		t.Errorf("reasoning = %#v", first.Message.Parts[0])
	}
	if first.Message.Parts[1].Text != "hel" || first.Message.Parts[3].Text != "lo" {
		t.Errorf("text boundaries = %#v", first.Message.Parts)
	}
	call := first.Message.Parts[2].ToolCall
	if call == nil || call.ID != "call-1" || call.Name != "search" || call.Arguments != `{"q":"lynx"}` {
		t.Errorf("tool call = %#v", call)
	}
	if first.FinishReason != chat.FinishReasonToolCalls || response.Choices[1].Text() != "alternative" {
		t.Errorf("finish/alternative = %q/%q", first.FinishReason, response.Choices[1].Text())
	}
	if response.Usage.InputTokens != 12 || response.Usage.OutputTokens != 5 {
		t.Errorf("usage = %#v", response.Usage)
	}
	if got := decode[string](t, response.Extensions, "test/value"); got != "last" {
		t.Errorf("response extension = %q", got)
	}
	if got := decode[bool](t, first.Extensions, "test/finish"); !got {
		t.Error("choice extension was not merged")
	}
	if err := response.Validate(); err != nil {
		t.Fatalf("Response.Validate: %v", err)
	}
}

func TestResponseAccumulatorMergesInterleavedParallelToolCalls(t *testing.T) {
	chunks := []*chat.Response{
		responseWithParts(
			chat.NewToolCallPart(chat.ToolCall{ID: "call-a", Name: "a", Arguments: `{"a":`}),
			chat.NewToolCallPart(chat.ToolCall{ID: "call-b", Name: "b", Arguments: `{"b":`}),
		),
		responseWithParts(
			chat.NewToolCallPart(chat.ToolCall{ID: "call-a", Name: "a", Arguments: `1}`}),
			chat.NewToolCallPart(chat.ToolCall{ID: "call-b", Name: "b", Arguments: `2}`}),
		),
	}
	var accumulator chat.ResponseAccumulator
	for _, chunk := range chunks {
		if err := accumulator.Add(chunk); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	parts := accumulator.Response().Choices[0].Message.Parts
	if len(parts) != 2 || parts[0].ToolCall.Arguments != `{"a":1}` || parts[1].ToolCall.Arguments != `{"b":2}` {
		t.Fatalf("parallel tools = %#v", parts)
	}
}

func TestResponseAccumulatorTreatsUsageAsLatestSnapshot(t *testing.T) {
	reasoning := int64(2)
	chunks := []*chat.Response{
		{Usage: chat.Usage{InputTokens: 8}},
		{},
		{Usage: chat.Usage{InputTokens: 8, OutputTokens: 3, ReasoningTokens: &reasoning}},
	}
	var accumulator chat.ResponseAccumulator
	for _, chunk := range chunks {
		if err := accumulator.Add(chunk); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	usage := accumulator.Response().Usage
	if usage.InputTokens != 8 || usage.OutputTokens != 3 || usage.ReasoningTokens == nil || *usage.ReasoningTokens != 2 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestResponseAccumulatorDoesNotAliasChunksOrSnapshots(t *testing.T) {
	chunk := responseWithParts(chat.NewTextPart("p"))
	continuation := responseWithParts(chat.NewTextPart("ong"))

	var first, second chat.ResponseAccumulator
	for _, accumulator := range []*chat.ResponseAccumulator{&first, &second} {
		if err := accumulator.Add(chunk); err != nil {
			t.Fatal(err)
		}
		if err := accumulator.Add(continuation); err != nil {
			t.Fatal(err)
		}
	}
	if chunk.Text() != "p" || continuation.Text() != "ong" {
		t.Fatalf("input chunks mutated to %q/%q", chunk.Text(), continuation.Text())
	}
	if first.Response().Text() != "pong" || second.Response().Text() != "pong" {
		t.Fatalf("accumulated text = %q/%q", first.Response().Text(), second.Response().Text())
	}

	snapshot := first.Response()
	snapshot.Choices[0].Message.Parts[0].Text = "mutated"
	if got := first.Response().Text(); got != "pong" {
		t.Fatalf("snapshot mutation changed accumulator to %q", got)
	}
}

func TestResponseAccumulatorClonesMediaAndMessageMetadata(t *testing.T) {
	image, err := media.NewBytes("image/png", []byte("image"))
	if err != nil {
		t.Fatal(err)
	}
	message := assistant(chat.NewMediaPart(image))
	message.Metadata = metadata.New()
	if err := metadata.Set(message.Metadata, "test/value", "original"); err != nil {
		t.Fatal(err)
	}
	chunk := &chat.Response{Choices: []chat.Choice{{Index: 0, Message: message}}}

	var accumulator chat.ResponseAccumulator
	if err := accumulator.Add(chunk); err != nil {
		t.Fatal(err)
	}
	snapshot := accumulator.Response()
	snapshotMedia := snapshot.Choices[0].Message.Parts[0].Media
	snapshotMedia.Source.Bytes[0] = 'X'
	snapshot.Choices[0].Message.Metadata["test/value"][0] = 'X'

	got := accumulator.Response().Choices[0].Message
	if string(got.Parts[0].Media.Source.Bytes) != "image" || decode[string](t, got.Metadata, "test/value") != "original" {
		t.Fatalf("snapshot aliases accumulator: %#v", got)
	}
	if string(image.Source.Bytes) != "image" || decode[string](t, message.Metadata, "test/value") != "original" {
		t.Fatalf("accumulator aliases input: %#v", message)
	}
}

func TestResponseAccumulatorRejectsConflictingToolIdentityAtomically(t *testing.T) {
	var accumulator chat.ResponseAccumulator
	if err := accumulator.Add(responseWithParts(chat.NewToolCallPart(chat.ToolCall{
		ID: "call-1", Name: "search", Arguments: "{",
	}))); err != nil {
		t.Fatal(err)
	}
	err := accumulator.Add(responseWithParts(chat.NewToolCallPart(chat.ToolCall{
		ID: "call-1", Name: "lookup", Arguments: "}",
	})))
	if err == nil {
		t.Fatal("Add accepted conflicting tool name")
	}
	call := accumulator.Response().Choices[0].Message.Parts[0].ToolCall
	if call.Name != "search" || call.Arguments != "{" {
		t.Fatalf("failed Add mutated accumulator: %#v", call)
	}
}

func TestResponseAccumulatorNilAndZeroBehavior(t *testing.T) {
	var accumulator chat.ResponseAccumulator
	if accumulator.Response() != nil {
		t.Fatal("empty accumulator returned a response")
	}
	if err := accumulator.Add(nil); !errors.Is(err, chat.ErrInvalidResponse) {
		t.Fatalf("Add(nil) error = %v; want ErrInvalidResponse", err)
	}
	invalid := &chat.Response{Choices: []chat.Choice{{Index: -1, FinishReason: chat.FinishReasonStop}}}
	if err := accumulator.Add(invalid); !errors.Is(err, chat.ErrInvalidResponse) {
		t.Fatalf("Add(invalid) error = %v; want ErrInvalidResponse", err)
	}
	if accumulator.Response() != nil {
		t.Fatal("failed Add changed empty accumulator")
	}
	if err := accumulator.Add(&chat.Response{}); err != nil {
		t.Fatalf("Add zero response: %v", err)
	}
	if response := accumulator.Response(); response == nil || response.ID != "" || len(response.Choices) != 0 {
		t.Fatalf("zero response snapshot = %#v", response)
	}
	var nilAccumulator *chat.ResponseAccumulator
	if err := nilAccumulator.Add(&chat.Response{}); err == nil {
		t.Fatal("nil accumulator accepted Add")
	}
}

func assistant(parts ...chat.Part) *chat.Message {
	message := chat.NewAssistantMessage(parts...)
	return &message
}

func responseWithParts(parts ...chat.Part) *chat.Response {
	return &chat.Response{Choices: []chat.Choice{{Index: 0, Message: assistant(parts...)}}}
}

func partKinds(message *chat.Message) []chat.PartKind {
	if message == nil {
		return nil
	}
	kinds := make([]chat.PartKind, len(message.Parts))
	for i := range message.Parts {
		kinds[i] = message.Parts[i].Kind
	}
	return kinds
}

func decode[T any](t *testing.T, values metadata.Map, key string) T {
	t.Helper()
	value, found, err := metadata.Decode[T](values, key)
	if err != nil {
		t.Fatalf("Decode %q: %v", key, err)
	}
	if !found {
		t.Fatalf("metadata %q not found", key)
	}
	return value
}
