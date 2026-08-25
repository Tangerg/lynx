package chat_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/metadata"
)

func assistantResult(text string) *chat.Result {
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	return &chat.Result{Message: &message, FinishReason: chat.FinishReasonStop}
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
		t.Fatal("provider-native reason must map to Other plus result metadata")
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

func TestResultValidate(t *testing.T) {
	metadataOnly := &chat.ResultMetadata{}
	valid := []*chat.Result{
		assistantResult("hello"),
		{FinishReason: chat.FinishReasonStop},
		{Metadata: metadataOnly},
	}
	for i := range valid {
		if err := valid[i].Validate(); err != nil {
			t.Errorf("valid[%d]: %v", i, err)
		}
	}
}

func TestResultValidateRejectsInvalidValues(t *testing.T) {
	user := chat.NewUserMessage(chat.NewTextPart("hello"))
	invalidMessage := chat.Message{Role: chat.RoleAssistant}
	tests := []struct {
		name   string
		result *chat.Result
		also   error
	}{
		{name: "nil", result: nil},
		{name: "empty", result: &chat.Result{}},
		{name: "invalid message", result: &chat.Result{Message: &invalidMessage}, also: chat.ErrInvalidMessage},
		{name: "user message", result: &chat.Result{Message: &user}},
		{name: "unknown finish", result: &chat.Result{FinishReason: "future"}},
		{name: "invalid metadata", result: &chat.Result{Metadata: &chat.ResultMetadata{Extra: metadata.Map{"bad": json.RawMessage(`{`)}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.result.Validate()
			if !errors.Is(err, chat.ErrInvalidResponse) {
				t.Fatalf("Validate error = %v, want ErrInvalidResponse", err)
			}
			if tt.also != nil && !errors.Is(err, tt.also) {
				t.Fatalf("Validate error = %v, also want %v", err, tt.also)
			}
		})
	}
}

func TestResultHelpersAndJSON(t *testing.T) {
	result := assistantResult("hello")
	result.Metadata = &chat.ResultMetadata{}
	if result.Text() != "hello" {
		t.Fatalf("Text = %q", result.Text())
	}
	if err := result.Metadata.Set("openai/logprobs", []float64{-0.1}); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var got chat.Result
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, *result) {
		t.Fatalf("round trip = %#v, want %#v", got, *result)
	}

	var nilResult *chat.Result
	if nilResult.Text() != "" {
		t.Fatal("nil Result.Text must be empty")
	}
}

func TestResultUnmarshalIsAtomic(t *testing.T) {
	result := assistantResult("keep")
	if err := json.Unmarshal([]byte(`{"finish_reason":"future"}`), result); !errors.Is(err, chat.ErrInvalidResponse) {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if result.Text() != "keep" {
		t.Fatalf("failed Unmarshal mutated result: %+v", result)
	}
	var nilResult *chat.Result
	if err := nilResult.UnmarshalJSON([]byte(`{}`)); !errors.Is(err, chat.ErrInvalidResponse) {
		t.Fatalf("nil UnmarshalJSON error = %v", err)
	}
}

func TestResponseCloneOwnsNestedProtocolValues(t *testing.T) {
	reasoningTokens := int64(2)
	response, err := chat.NewResponse(assistantResult("original"), &chat.ResponseMetadata{
		Model: "model",
		Usage: chat.Usage{OutputTokens: 2, ReasoningTokens: &reasoningTokens},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Metadata.Set("test/value", map[string]int{"count": 1}); err != nil {
		t.Fatal(err)
	}

	cloned := response.Clone()
	cloned.Metadata.Model = "mutated"
	cloned.Result.Message.Parts[0].Text = "mutated"
	*cloned.Metadata.Usage.ReasoningTokens = 9
	cloned.Metadata.Extra["test/value"][0] = 'x'

	if response.Metadata.Model != "model" ||
		response.Text() != "original" ||
		*response.Metadata.Usage.ReasoningTokens != 2 ||
		response.Metadata.Extra["test/value"][0] == 'x' {
		t.Fatalf("mutating clone changed source response: %#v", response)
	}
	var nilResponse *chat.Response
	if nilResponse.Clone() != nil {
		t.Fatal("nil Response.Clone returned a non-nil value")
	}
}

func TestResponseZeroAndNilHelpers(t *testing.T) {
	response := &chat.Response{}
	if response.Result != nil || response.Text() != "" {
		t.Fatal("empty response helpers must be nil/empty")
	}
	encoded, err := json.Marshal(response)
	if err != nil || string(encoded) != `{}` {
		t.Fatalf("zero Response JSON = %s, %v", encoded, err)
	}
	var nilResponse *chat.Response
	if nilResponse.Text() != "" {
		t.Fatal("nil response text must be empty")
	}
}

func TestResponseValidateRejectsInvalidValues(t *testing.T) {
	invalidUsage := chat.Usage{InputTokens: -1}
	tests := []struct {
		name     string
		response *chat.Response
		also     error
	}{
		{name: "nil", response: nil},
		{name: "ID whitespace", response: &chat.Response{Metadata: &chat.ResponseMetadata{ID: " id"}}},
		{name: "model whitespace", response: &chat.Response{Metadata: &chat.ResponseMetadata{Model: "model "}}},
		{name: "invalid result", response: &chat.Response{Result: &chat.Result{}}},
		{name: "invalid usage", response: &chat.Response{Metadata: &chat.ResponseMetadata{Usage: invalidUsage}}, also: chat.ErrInvalidUsage},
		{name: "invalid metadata", response: &chat.Response{Metadata: &chat.ResponseMetadata{Extra: metadata.Map{"bad": json.RawMessage(`{`)}}}},
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
	response, err := chat.NewResponse(assistantResult("hello"), &chat.ResponseMetadata{
		ID:    "response-1",
		Model: "model",
		Usage: chat.Usage{InputTokens: 10, OutputTokens: 6, ReasoningTokens: &reasoning, CacheReadInputTokens: &cacheRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Metadata.Set("openai/system_fingerprint", "fp-1"); err != nil {
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

func TestResponseUnmarshalIsAtomic(t *testing.T) {
	response := &chat.Response{Metadata: &chat.ResponseMetadata{ID: "keep"}}
	if err := json.Unmarshal([]byte(`{"result":{}}`), response); !errors.Is(err, chat.ErrInvalidResponse) {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if response.Metadata.ID != "keep" {
		t.Fatalf("failed Unmarshal mutated response: %+v", response)
	}
	var nilResponse *chat.Response
	if err := nilResponse.UnmarshalJSON([]byte(`{}`)); !errors.Is(err, chat.ErrInvalidResponse) {
		t.Fatalf("nil UnmarshalJSON error = %v", err)
	}
}

func TestResponseProtocolFieldsExcludeToolLoopState(t *testing.T) {
	typ := reflect.TypeFor[chat.Response]()
	want := []string{"Result", "Metadata"}
	if typ.NumField() != len(want) {
		t.Fatalf("Response has %d fields, want %d", typ.NumField(), len(want))
	}
	for i, name := range want {
		if typ.Field(i).Name != name {
			t.Errorf("Response field[%d] = %s, want %s", i, typ.Field(i).Name, name)
		}
	}
}
