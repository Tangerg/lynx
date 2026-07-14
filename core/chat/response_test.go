package chat_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/metadata"
)

func assistantChoice(index int, text string) chat.Choice {
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	return chat.Choice{Index: index, Message: &message, FinishReason: chat.FinishReasonStop}
}

func TestFinishReason(t *testing.T) {
	for _, reason := range []chat.FinishReason{
		"",
		chat.FinishReasonStop,
		chat.FinishReasonLength,
		chat.FinishReasonToolCalls,
		chat.FinishReasonContentFilter,
		chat.FinishReasonOther,
	} {
		if !reason.Valid() {
			t.Errorf("%q must be valid", reason)
		}
		if reason.String() != string(reason) {
			t.Errorf("String(%q) = %q", reason, reason.String())
		}
	}
	if chat.FinishReason("provider-native").Valid() {
		t.Fatal("provider-native reason must map to Other plus an extension")
	}
}

func TestMessageText(t *testing.T) {
	message := chat.NewAssistantMessage(
		chat.NewTextPart("hello "),
		chat.NewReasoningPart("hidden", nil),
		chat.NewTextPart("world"),
	)
	if got := message.Text(); got != "hello world" {
		t.Fatalf("Text = %q, want hello world", got)
	}
	var nilMessage *chat.Message
	if nilMessage.Text() != "" {
		t.Fatal("nil Message.Text must be empty")
	}
}

func TestChoiceValidate(t *testing.T) {
	valid := []chat.Choice{
		assistantChoice(0, "hello"),
		{Index: 0, FinishReason: chat.FinishReasonStop},
		{Index: 0, Extensions: metadata.Map{"openai/logprobs": json.RawMessage(`[]`)}},
	}
	for i := range valid {
		if err := valid[i].Validate(); err != nil {
			t.Errorf("valid[%d]: %v", i, err)
		}
	}
}

func TestChoiceValidateRejectsInvalidValues(t *testing.T) {
	user := chat.NewUserMessage(chat.NewTextPart("hello"))
	invalidMessage := chat.Message{Role: chat.RoleAssistant}
	tests := []struct {
		name   string
		choice *chat.Choice
		also   error
	}{
		{name: "nil", choice: nil},
		{name: "negative index", choice: &chat.Choice{Index: -1, FinishReason: chat.FinishReasonStop}},
		{name: "empty", choice: &chat.Choice{}},
		{name: "invalid message", choice: &chat.Choice{Message: &invalidMessage}, also: chat.ErrInvalidMessage},
		{name: "user message", choice: &chat.Choice{Message: &user}},
		{name: "unknown finish", choice: &chat.Choice{FinishReason: "future"}},
		{name: "invalid extension", choice: &chat.Choice{Extensions: metadata.Map{"bad": json.RawMessage(`1`)}}, also: chat.ErrInvalidExtension},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.choice.Validate()
			if !errors.Is(err, chat.ErrInvalidChoice) {
				t.Fatalf("Validate error = %v, want ErrInvalidChoice", err)
			}
			if tt.also != nil && !errors.Is(err, tt.also) {
				t.Fatalf("Validate error = %v, also want %v", err, tt.also)
			}
		})
	}
}

func TestChoiceHelpersAndJSON(t *testing.T) {
	choice := assistantChoice(2, "hello")
	if choice.Text() != "hello" {
		t.Fatalf("Text = %q", choice.Text())
	}
	if err := choice.SetExtension("openai/logprobs", []float64{-0.1}); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(choice)
	if err != nil {
		t.Fatal(err)
	}
	var got chat.Choice
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, choice) {
		t.Fatalf("round trip = %#v, want %#v", got, choice)
	}

	var nilChoice *chat.Choice
	if nilChoice.Text() != "" {
		t.Fatal("nil Choice.Text must be empty")
	}
	if err := nilChoice.SetExtension("openai/key", 1); !errors.Is(err, chat.ErrInvalidChoice) {
		t.Fatalf("nil SetExtension error = %v", err)
	}
}

func TestChoiceUnmarshalIsAtomic(t *testing.T) {
	choice := assistantChoice(0, "keep")
	if err := json.Unmarshal([]byte(`{"index":-1,"finish_reason":"stop"}`), &choice); !errors.Is(err, chat.ErrInvalidChoice) {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if choice.Text() != "keep" {
		t.Fatalf("failed Unmarshal mutated choice: %+v", choice)
	}
	var nilChoice *chat.Choice
	if err := nilChoice.UnmarshalJSON([]byte(`{}`)); !errors.Is(err, chat.ErrInvalidChoice) {
		t.Fatalf("nil UnmarshalJSON error = %v", err)
	}
}

func TestResponsePreservesAllChoices(t *testing.T) {
	choices := []chat.Choice{assistantChoice(0, "first"), assistantChoice(1, "second"), assistantChoice(2, "third")}
	response, err := chat.NewResponse(choices...)
	if err != nil {
		t.Fatal(err)
	}
	choices[0] = assistantChoice(0, "mutated")
	if len(response.Choices) != 3 || response.Choices[0].Text() != "first" || response.Choices[2].Text() != "third" {
		t.Fatalf("choices not preserved: %+v", response.Choices)
	}
	if response.First() != &response.Choices[0] {
		t.Fatal("First must return the first stored choice")
	}
	if response.Text() != "first" {
		t.Fatalf("Text = %q, want first", response.Text())
	}
}

func TestResponseZeroAndNilHelpers(t *testing.T) {
	response, err := chat.NewResponse()
	if err != nil {
		t.Fatal(err)
	}
	if response.First() != nil || response.Text() != "" {
		t.Fatal("empty response helpers must be nil/empty")
	}
	encoded, err := json.Marshal(response)
	if err != nil || string(encoded) != `{}` {
		t.Fatalf("zero Response JSON = %s, %v", encoded, err)
	}
	var nilResponse *chat.Response
	if nilResponse.First() != nil || nilResponse.Text() != "" {
		t.Fatal("nil response helpers must be nil/empty")
	}
}

func TestResponseValidateRejectsInvalidValues(t *testing.T) {
	validChoice := assistantChoice(0, "hello")
	invalidUsage := chat.Usage{InputTokens: -1}
	tests := []struct {
		name     string
		response *chat.Response
		also     error
	}{
		{name: "nil", response: nil},
		{name: "ID whitespace", response: &chat.Response{ID: " id"}},
		{name: "model whitespace", response: &chat.Response{Model: "model "}},
		{name: "invalid choice", response: &chat.Response{Choices: []chat.Choice{{}}}, also: chat.ErrInvalidChoice},
		{name: "duplicate index", response: &chat.Response{Choices: []chat.Choice{validChoice, validChoice}}},
		{name: "invalid usage", response: &chat.Response{Usage: invalidUsage}, also: chat.ErrInvalidUsage},
		{name: "invalid extension", response: &chat.Response{Extensions: metadata.Map{"bad": json.RawMessage(`1`)}}, also: chat.ErrInvalidExtension},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.response.Validate()
			if !errors.Is(err, chat.ErrInvalidResponse) {
				t.Fatalf("Validate error = %v, want ErrInvalidResponse", err)
			}
			if tt.also != nil && !errors.Is(err, tt.also) {
				t.Fatalf("Validate error = %v, also want %v", err, tt.also)
			}
		})
	}
}

func TestResponseJSONRoundTrip(t *testing.T) {
	reasoning := int64(4)
	cacheRead := int64(3)
	response, _ := chat.NewResponse(assistantChoice(0, "hello"), assistantChoice(1, "alternate"))
	response.ID = "response-1"
	response.Model = "model"
	response.Usage = chat.Usage{InputTokens: 10, OutputTokens: 6, ReasoningTokens: &reasoning, CacheReadInputTokens: &cacheRead}
	if err := response.SetExtension("openai/system_fingerprint", "fp-1"); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var got chat.Response
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, *response) {
		t.Fatalf("round trip = %#v, want %#v", got, *response)
	}
}

func TestResponseSetExtensionAndUnmarshalAreSafe(t *testing.T) {
	response := &chat.Response{}
	if err := response.SetExtension("openai/usage", map[string]int{"cached": 3}); err != nil {
		t.Fatal(err)
	}
	before := response.Extensions.Clone()
	if err := response.SetExtension("bad", 1); !errors.Is(err, chat.ErrInvalidExtension) {
		t.Fatalf("invalid extension error = %v", err)
	}
	if !reflect.DeepEqual(response.Extensions, before) {
		t.Fatal("failed SetExtension mutated response")
	}

	response.ID = "keep"
	if err := json.Unmarshal([]byte(`{"choices":[{}]}`), response); !errors.Is(err, chat.ErrInvalidResponse) {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if response.ID != "keep" {
		t.Fatalf("failed Unmarshal mutated response: %+v", response)
	}
	var nilResponse *chat.Response
	if err := nilResponse.SetExtension("openai/key", 1); !errors.Is(err, chat.ErrInvalidResponse) {
		t.Fatalf("nil SetExtension error = %v", err)
	}
	if err := nilResponse.UnmarshalJSON([]byte(`{}`)); !errors.Is(err, chat.ErrInvalidResponse) {
		t.Fatalf("nil UnmarshalJSON error = %v", err)
	}
}

func TestResponseProtocolFieldsExcludeToolLoopState(t *testing.T) {
	typ := reflect.TypeFor[chat.Response]()
	want := []string{"ID", "Model", "Choices", "Usage", "Extensions"}
	if typ.NumField() != len(want) {
		t.Fatalf("Response has %d fields, want %d", typ.NumField(), len(want))
	}
	for i, name := range want {
		if typ.Field(i).Name != name {
			t.Errorf("Response field[%d] = %s, want %s", i, typ.Field(i).Name, name)
		}
	}
}
