package chat_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

func TestNewReasoningPartCopiesSignature(t *testing.T) {
	signature := []byte("signature")
	reasoning := chat.NewReasoningPart("thinking", signature)
	signature[0] = 'X'
	if string(reasoning.Signature) != "signature" {
		t.Fatalf("reasoning signature = %q", reasoning.Signature)
	}
}

func TestPartValidate(t *testing.T) {
	image := mustImage(t)
	valid := []chat.Part{
		chat.NewTextPart("hello"),
		chat.NewMediaPart(image),
		chat.NewReasoningPart("thinking", nil),
		chat.NewReasoningPart("", []byte("opaque")),
		chat.NewToolCallPart(validToolCall()),
		chat.NewToolResultPart(validToolResult()),
	}
	for _, part := range valid {
		if err := part.Validate(); err != nil {
			t.Errorf("Validate(%+v): %v", part, err)
		}
	}
}

func TestPartValidateRejectsInvalidAndAmbiguousValues(t *testing.T) {
	tests := []chat.Part{
		{},
		{Kind: "future", Text: "x"},
		{Kind: chat.PartText},
		{Kind: chat.PartText, Text: "x", Signature: []byte("extra")},
		{Kind: chat.PartMedia},
		{Kind: chat.PartReasoning},
		{Kind: chat.PartToolCall},
		{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{Name: "tool", Arguments: `[]`}},
		{Kind: chat.PartToolResult},
		{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{ID: "call"}},
	}
	for _, part := range tests {
		if err := part.Validate(); !errors.Is(err, chat.ErrInvalidPart) {
			t.Errorf("Validate(%+v) error = %v, want ErrInvalidPart", part, err)
		}
	}
}

func TestPartValidateRecursesIntoMedia(t *testing.T) {
	part := chat.NewMediaPart(&media.Media{})
	if err := part.Validate(); !errors.Is(err, media.ErrInvalidMIME) || !errors.Is(err, chat.ErrInvalidPart) {
		t.Fatalf("Validate error = %v, want media and part errors", err)
	}
}

func TestPartJSONRoundTrip(t *testing.T) {
	parts := []chat.Part{
		chat.NewTextPart("hello"),
		chat.NewMediaPart(mustImage(t)),
		chat.NewReasoningPart("thinking", []byte("signature")),
		chat.NewToolCallPart(validToolCall()),
		chat.NewToolResultPart(validToolResult()),
	}
	for _, part := range parts {
		encoded, err := json.Marshal(part)
		if err != nil {
			t.Fatalf("Marshal(%q): %v", part.Kind, err)
		}
		var got chat.Part
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatalf("Unmarshal(%q): %v", part.Kind, err)
		}
		if part.Kind == chat.PartMedia {
			if got.Kind != part.Kind || got.Media.MIME != part.Media.MIME || !reflect.DeepEqual(got.Media.Source, part.Media.Source) {
				t.Fatalf("media round trip = %#v, want %#v", got, part)
			}
			continue
		}
		if !reflect.DeepEqual(got, part) {
			t.Fatalf("round trip = %#v, want %#v", got, part)
		}
	}
}

func TestPartUnmarshalRejectsUnknownKindWithoutMutatingReceiver(t *testing.T) {
	got := chat.NewTextPart("keep")
	err := json.Unmarshal([]byte(`{"kind":"future","text":"replace"}`), &got)
	if !errors.Is(err, chat.ErrInvalidPart) {
		t.Fatalf("Unmarshal error = %v, want ErrInvalidPart", err)
	}
	if got.Kind != chat.PartText || got.Text != "keep" {
		t.Fatalf("failed Unmarshal mutated receiver: %+v", got)
	}
}

func TestPartNilUnmarshalReceiver(t *testing.T) {
	var part *chat.Part
	if err := part.UnmarshalJSON([]byte(`{}`)); !errors.Is(err, chat.ErrInvalidPart) {
		t.Fatalf("UnmarshalJSON error = %v, want ErrInvalidPart", err)
	}
}

func TestPartKindValid(t *testing.T) {
	for _, kind := range []chat.PartKind{chat.PartText, chat.PartMedia, chat.PartReasoning, chat.PartToolCall, chat.PartToolResult} {
		if !kind.Valid() {
			t.Errorf("%q must be valid", kind)
		}
	}
	if chat.PartKind("future").Valid() {
		t.Fatal("future kind must be invalid")
	}
}
