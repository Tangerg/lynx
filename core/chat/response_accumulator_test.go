package chat_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
)

func TestResponseAccumulatorAggregatesResultDeltas(t *testing.T) {
	chunks := []*chat.Response{
		{
			Result: responseResult(chat.NewReasoningPart("step ", []byte("sig-"))),
			Metadata: &chat.ResponseMetadata{
				ID:    "response-1",
				Model: "model-initial",
			},
		},
		{
			Result: responseResult(
				chat.NewReasoningPart("one", []byte("nature")),
				chat.NewTextPart("hel"),
				chat.NewToolCallPart(chat.ToolCall{ID: "call-1", Name: "search", Arguments: `{"q":"`}),
			),
		},
		{
			Result: &chat.Result{
				Message: assistant(
					chat.NewToolCallPart(chat.ToolCall{ID: "call-1", Name: "search", Arguments: `lynx"}`}),
					chat.NewTextPart("lo"),
				),
				FinishReason: chat.FinishReasonToolCalls,
				Metadata:     &chat.ResultMetadata{},
			},
			Metadata: &chat.ResponseMetadata{
				Model: "model-final",
				Usage: chat.Usage{InputTokens: 12, OutputTokens: 5},
			},
		},
	}
	chunks[0].Metadata.Extra = metadata.Map{}
	if err := chunks[0].Metadata.Set("test/value", "first"); err != nil {
		t.Fatal(err)
	}
	if err := chunks[2].Metadata.Set("test/value", "last"); err != nil {
		t.Fatal(err)
	}
	if err := chunks[2].Result.Metadata.Set("test/finish", true); err != nil {
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
	if response.Metadata.ID != "response-1" || response.Metadata.Model != "model-final" {
		t.Errorf("identity = %q/%q", response.Metadata.ID, response.Metadata.Model)
	}
	wantKinds := []chat.PartKind{chat.PartReasoning, chat.PartText, chat.PartToolCall, chat.PartText}
	if got := partKinds(response.Result.Message); !slices.Equal(got, wantKinds) {
		t.Fatalf("part kinds = %v; want %v", got, wantKinds)
	}
	if response.Result.Message.Parts[0].Text != "step one" || string(response.Result.Message.Parts[0].Signature) != "sig-nature" {
		t.Errorf("reasoning = %#v", response.Result.Message.Parts[0])
	}
	if response.Result.Message.Parts[1].Text != "hel" || response.Result.Message.Parts[3].Text != "lo" {
		t.Errorf("text boundaries = %#v", response.Result.Message.Parts)
	}
	call := response.Result.Message.Parts[2].ToolCall
	if call == nil || call.ID != "call-1" || call.Name != "search" || call.Arguments != `{"q":"lynx"}` {
		t.Errorf("tool call = %#v", call)
	}
	if response.Result.FinishReason != chat.FinishReasonToolCalls {
		t.Errorf("finish = %q", response.Result.FinishReason)
	}
	if response.Metadata.Usage.InputTokens != 12 || response.Metadata.Usage.OutputTokens != 5 {
		t.Errorf("usage = %#v", response.Metadata.Usage)
	}
	if got := decode[string](t, response.Metadata.Extra, "test/value"); got != "last" {
		t.Errorf("response metadata = %q", got)
	}
	if got := decode[bool](t, response.Result.Metadata.Extra, "test/finish"); !got {
		t.Error("result metadata was not merged")
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
	parts := accumulator.Response().Result.Message.Parts
	if len(parts) != 2 || parts[0].ToolCall.Arguments != `{"a":1}` || parts[1].ToolCall.Arguments != `{"b":2}` {
		t.Fatalf("parallel tools = %#v", parts)
	}
}

func TestResponseAccumulatorTreatsUsageAsLatestSnapshot(t *testing.T) {
	reasoning := int64(2)
	chunks := []*chat.Response{
		{Metadata: &chat.ResponseMetadata{Usage: chat.Usage{InputTokens: 8}}},
		{},
		{Metadata: &chat.ResponseMetadata{Usage: chat.Usage{InputTokens: 8, OutputTokens: 3, ReasoningTokens: &reasoning}}},
	}
	var accumulator chat.ResponseAccumulator
	for _, chunk := range chunks {
		if err := accumulator.Add(chunk); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	usage := accumulator.Response().Metadata.Usage
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
	snapshot.Result.Message.Parts[0].Text = "mutated"
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
	message.Metadata = metadata.Map{}
	if err := message.Metadata.Set("test/value", "original"); err != nil {
		t.Fatal(err)
	}
	chunk := &chat.Response{Result: &chat.Result{Message: message}}

	var accumulator chat.ResponseAccumulator
	if err := accumulator.Add(chunk); err != nil {
		t.Fatal(err)
	}
	snapshot := accumulator.Response()
	snapshotMedia := snapshot.Result.Message.Parts[0].Media
	snapshotMedia.Source.Bytes[0] = 'X'
	snapshot.Result.Message.Metadata["test/value"][0] = 'X'

	got := accumulator.Response().Result.Message
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
	call := accumulator.Response().Result.Message.Parts[0].ToolCall
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
	invalid := &chat.Response{Result: &chat.Result{}}
	if err := accumulator.Add(invalid); !errors.Is(err, chat.ErrInvalidResponse) {
		t.Fatalf("Add(invalid) error = %v; want ErrInvalidResponse", err)
	}
	if accumulator.Response() != nil {
		t.Fatal("failed Add changed empty accumulator")
	}
	if err := accumulator.Add(&chat.Response{}); err != nil {
		t.Fatalf("Add zero response: %v", err)
	}
	if response := accumulator.Response(); response == nil || response.Result != nil || response.Metadata != nil {
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

func responseResult(parts ...chat.Part) *chat.Result {
	return &chat.Result{Message: assistant(parts...)}
}

func responseWithParts(parts ...chat.Part) *chat.Response {
	return &chat.Response{Result: responseResult(parts...)}
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
