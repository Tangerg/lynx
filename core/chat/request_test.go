package chat_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
)

func validToolDefinition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "weather",
		Description: "look up weather",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
	}
}

func TestNewRequest(t *testing.T) {
	messages := []chat.Message{chat.NewUserMessage(chat.NewTextPart("hello"))}
	request, err := chat.NewRequest(messages...)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	messages[0] = chat.NewSystemMessage("changed")
	if request.Messages[0].Role != chat.RoleUser {
		t.Fatal("NewRequest retained the caller's message slice")
	}
	if !reflect.DeepEqual(request.Options, chat.Options{}) {
		t.Fatalf("Options = %+v, want legal zero value", request.Options)
	}
}

func TestRequestClone(t *testing.T) {
	image, err := media.NewBytes("image/png", []byte("image"))
	if err != nil {
		t.Fatal(err)
	}
	if err := image.Metadata.Set("origin", "caller"); err != nil {
		t.Fatal(err)
	}
	user := chat.NewUserMessage(chat.NewMediaPart(image))
	if err := user.Metadata.Set("turn", 1); err != nil {
		t.Fatal(err)
	}
	assistant := chat.NewAssistantMessage(
		chat.NewReasoningPart("thinking", []byte("signature")),
		chat.NewToolCallPart(validToolCall()),
	)
	tool := chat.NewToolMessage(validToolResult())

	request := &chat.Request{
		Messages: []chat.Message{user, assistant, tool},
		Tools:    []chat.ToolDefinition{validToolDefinition()},
		Options: chat.Options{
			MaxTokens:   new(int64(10)),
			Stop:        []string{"END"},
			Temperature: new(0.5),
		},
	}
	if err := request.SetExtension("test/value", "caller"); err != nil {
		t.Fatal(err)
	}

	clone := request.Clone()
	clone.Messages[0].Metadata["turn"][0] = '9'
	clone.Messages[0].Parts[0].Media.Source.Bytes[0] = 'X'
	clone.Messages[0].Parts[0].Media.Metadata["origin"][1] = 'X'
	clone.Messages[1].Parts[0].Signature[0] = 'X'
	clone.Messages[1].Parts[1].ToolCall.Name = "mutated"
	clone.Messages[2].Parts[0].ToolResult.Result = "mutated"
	clone.Tools[0].InputSchema[0] = '['
	*clone.Options.MaxTokens = 20
	clone.Options.Stop[0] = "MUTATED"
	*clone.Options.Temperature = 1
	clone.Extensions["test/value"][1] = 'X'

	if string(request.Messages[0].Metadata["turn"]) != "1" ||
		string(request.Messages[0].Parts[0].Media.Source.Bytes) != "image" ||
		string(request.Messages[0].Parts[0].Media.Metadata["origin"]) != `"caller"` ||
		string(request.Messages[1].Parts[0].Signature) != "signature" ||
		request.Messages[1].Parts[1].ToolCall.Name != "weather" ||
		request.Messages[2].Parts[0].ToolResult.Result != `{"temperature":20}` ||
		request.Tools[0].InputSchema[0] != '{' ||
		*request.Options.MaxTokens != 10 ||
		request.Options.Stop[0] != "END" ||
		*request.Options.Temperature != 0.5 ||
		string(request.Extensions["test/value"]) != `"caller"` {
		t.Fatalf("clone mutated source request: %#v", request)
	}

	var nilRequest *chat.Request
	if nilRequest.Clone() != nil {
		t.Fatal("nil Request.Clone must return nil")
	}
}

func TestRequestValidate(t *testing.T) {
	request, err := chat.NewRequest(chat.NewSystemMessage("rules"), chat.NewUserMessage(chat.NewTextPart("hello")))
	if err != nil {
		t.Fatal(err)
	}
	request.Tools = []chat.ToolDefinition{validToolDefinition()}
	request.Options = chat.Options{Model: "model", Temperature: new(0.5)}
	if err := request.SetExtension("openai/request", map[string]any{"response_format": "json"}); err != nil {
		t.Fatal(err)
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestRequestValidateRejectsInvalidValues(t *testing.T) {
	validMessage := chat.NewUserMessage(chat.NewTextPart("hello"))
	invalidMessage := chat.Message{Role: chat.RoleUser}
	invalidOption := chat.Options{Temperature: new(3.0)}
	definition := validToolDefinition()

	tests := []struct {
		name    string
		request *chat.Request
		also    error
	}{
		{name: "nil", request: nil},
		{name: "no messages", request: &chat.Request{}},
		{name: "invalid message", request: &chat.Request{Messages: []chat.Message{invalidMessage}}, also: chat.ErrInvalidMessage},
		{name: "invalid tool", request: &chat.Request{Messages: []chat.Message{validMessage}, Tools: []chat.ToolDefinition{{}}}, also: chat.ErrInvalidToolDefinition},
		{name: "duplicate tool", request: &chat.Request{Messages: []chat.Message{validMessage}, Tools: []chat.ToolDefinition{definition, definition}}},
		{name: "invalid options", request: &chat.Request{Messages: []chat.Message{validMessage}, Options: invalidOption}, also: chat.ErrInvalidOptions},
		{name: "unscoped extension", request: &chat.Request{Messages: []chat.Message{validMessage}, Extensions: metadata.Map{"key": json.RawMessage(`1`)}}, also: chat.ErrInvalidExtension},
		{name: "invalid extension JSON", request: &chat.Request{Messages: []chat.Message{validMessage}, Extensions: metadata.Map{"openai/key": json.RawMessage(`{`)}}, also: metadata.ErrInvalidValue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if !errors.Is(err, chat.ErrInvalidRequest) {
				t.Fatalf("Validate error = %v, want ErrInvalidRequest", err)
			}
			if tt.also != nil && !errors.Is(err, tt.also) {
				t.Fatalf("Validate error = %v, also want %v", err, tt.also)
			}
		})
	}
}

func TestRequestSetExtension(t *testing.T) {
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("hello")))
	if err := request.SetExtension("openai/response_format", map[string]string{"type": "json_object"}); err != nil {
		t.Fatalf("SetExtension: %v", err)
	}
	value, ok, err := metadata.Decode[map[string]string](request.Extensions, "openai/response_format")
	if err != nil || !ok || value["type"] != "json_object" {
		t.Fatalf("Decode extension = (%v, %v, %v)", value, ok, err)
	}

	before := request.Extensions.Clone()
	if err := request.SetExtension("not-namespaced", 1); !errors.Is(err, chat.ErrInvalidExtension) {
		t.Fatalf("unscoped key error = %v", err)
	}
	if err := request.SetExtension("openai/bad", func() {}); !errors.Is(err, chat.ErrInvalidExtension) {
		t.Fatalf("unsupported value error = %v", err)
	}
	if !reflect.DeepEqual(request.Extensions, before) {
		t.Fatalf("failed SetExtension mutated map: %#v, want %#v", request.Extensions, before)
	}

	var nilRequest *chat.Request
	if err := nilRequest.SetExtension("openai/key", 1); !errors.Is(err, chat.ErrInvalidRequest) {
		t.Fatalf("nil request error = %v", err)
	}
}

func TestRequestJSONRoundTrip(t *testing.T) {
	request, _ := chat.NewRequest(
		chat.NewSystemMessage("rules"),
		chat.NewAssistantMessage(chat.NewToolCallPart(validToolCall())),
	)
	request.Tools = []chat.ToolDefinition{validToolDefinition()}
	request.Options = chat.Options{Model: "model", MaxTokens: new(int64(100))}
	if err := request.SetExtension("anthropic/cache_control", map[string]bool{"enabled": true}); err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"extensions"`) || !strings.Contains(string(encoded), `"input_schema"`) {
		t.Fatalf("request JSON missing protocol fields: %s", encoded)
	}
	var got chat.Request
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got, *request) {
		t.Fatalf("round trip = %#v, want %#v", got, *request)
	}
}

func TestRequestOmitsZeroOptions(t *testing.T) {
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("hello")))
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"options"`) {
		t.Fatalf("zero Options must be omitted: %s", encoded)
	}
}

func TestRequestUnmarshalIsAtomic(t *testing.T) {
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("keep")))
	err := json.Unmarshal([]byte(`{"messages":[]}`), request)
	if !errors.Is(err, chat.ErrInvalidRequest) {
		t.Fatalf("Unmarshal error = %v, want ErrInvalidRequest", err)
	}
	if request.Messages[0].Parts[0].Text != "keep" {
		t.Fatalf("failed Unmarshal mutated request: %+v", request)
	}
}

func TestRequestNilUnmarshalReceiver(t *testing.T) {
	var request *chat.Request
	if err := request.UnmarshalJSON([]byte(`{}`)); !errors.Is(err, chat.ErrInvalidRequest) {
		t.Fatalf("UnmarshalJSON error = %v, want ErrInvalidRequest", err)
	}
}

func TestRequestProtocolFieldsContainNoInterfaces(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeFor[chat.Request](),
		reflect.TypeFor[chat.Options](),
		reflect.TypeFor[chat.ToolDefinition](),
	} {
		for i := range typ.NumField() {
			field := typ.Field(i)
			if field.Type.Kind() == reflect.Interface {
				t.Errorf("%s.%s is an interface field", typ, field.Name)
			}
		}
	}
}
