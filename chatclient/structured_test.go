package chatclient

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

func TestCallStructuredInjectsInstructionsAndPreservesRequest(t *testing.T) {
	response := responseWithText(t, `{"name":"tea","steps":["steep"]}`)
	model := callOnly{call: func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if got := request.Messages[len(request.Messages)-1].Text(); !strings.HasPrefix(got, "Make tea\n\nRespond with only RFC 8259-compliant JSON") {
			t.Fatalf("model user text = %q", got)
		}
		return response, nil
	}}
	client, err := New(model)
	if err != nil {
		t.Fatal(err)
	}
	request := textRequest("Make tea")

	got, raw, err := CallStructured(context.Background(), client, request, JSON[recipe]())
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "tea" || len(got.Steps) != 1 || raw != response {
		t.Fatalf("CallStructured = (%#v, %p), want decoded recipe and %p", got, raw, response)
	}
	if len(request.Messages[0].Parts) != 1 || request.Messages[0].Text() != "Make tea" {
		t.Fatalf("caller request mutated: %#v", request)
	}
}

func TestCallStructuredPreservesResponseOnDecodeAndCallErrors(t *testing.T) {
	decodeResponse := responseWithText(t, "not JSON")
	client, err := New(callOnly{call: func(context.Context, *chat.Request) (*chat.Response, error) {
		return decodeResponse, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	value, raw, err := CallStructured(context.Background(), client, textRequest("hello"), JSON[recipe]())
	if err == nil || raw != decodeResponse || value.Name != "" || value.Steps != nil {
		t.Fatalf("decode failure = (%#v, %p, %v), want zero, response, error", value, raw, err)
	}

	callResponse := responseWithText(t, `{"name":"unused"}`)
	callError := errors.New("provider failed")
	client, err = New(callOnly{call: func(context.Context, *chat.Request) (*chat.Response, error) {
		return callResponse, callError
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, raw, err = CallStructured(context.Background(), client, textRequest("hello"), JSON[recipe]())
	if raw != callResponse || !errors.Is(err, callError) {
		t.Fatalf("call failure = (%p, %v), want response and provider error", raw, err)
	}
}

func TestCallStructuredValidatesBoundariesBeforeModel(t *testing.T) {
	var calls atomic.Int64
	client, err := New(callOnly{call: func(context.Context, *chat.Request) (*chat.Response, error) {
		calls.Add(1)
		return responseWithText(t, `{}`), nil
	}})
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := CallStructured[recipe](context.Background(), nil, textRequest("hello"), JSON[recipe]()); !errors.Is(err, ErrNilClient) {
		t.Fatalf("nil client error = %v", err)
	}
	if _, _, err := CallStructured(context.Background(), client, textRequest("hello"), Output[recipe]{}); !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("invalid output error = %v", err)
	}
	if _, _, err := CallStructured(context.Background(), client, nil, JSON[recipe]()); !errors.Is(err, chat.ErrInvalidRequest) {
		t.Fatalf("nil request error = %v", err)
	}
	systemOnly, err := chat.NewRequest(chat.NewSystemMessage("system"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := CallStructured(context.Background(), client, systemOnly, JSON[recipe]()); !errors.Is(err, ErrNoUserMessage) {
		t.Fatalf("no user error = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("model called %d times for invalid boundaries", calls.Load())
	}
}

func TestCallStructuredAllowsDecodeOnlyOutputWithoutUserMessage(t *testing.T) {
	client, err := New(callOnly{call: func(context.Context, *chat.Request) (*chat.Response, error) {
		return responseWithText(t, "plain"), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	request, err := chat.NewRequest(chat.NewSystemMessage("system"))
	if err != nil {
		t.Fatal(err)
	}
	output := Output[string]{Decode: func(value string) (string, error) { return strings.ToUpper(value), nil }}
	got, _, err := CallStructured(context.Background(), client, request, output)
	if err != nil || got != "PLAIN" {
		t.Fatalf("CallStructured = (%q, %v), want PLAIN", got, err)
	}
}

func TestCallStructuredAppendsInstructionsToMediaOnlyUser(t *testing.T) {
	image, err := mediaForStructuredTest()
	if err != nil {
		t.Fatal(err)
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewMediaPart(image)))
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(callOnly{call: func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if got := request.Messages[0].Text(); got != "plain instructions" {
			t.Fatalf("media-only instructions = %q", got)
		}
		return responseWithText(t, "ok"), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	output := Output[string]{
		Instructions: "plain instructions",
		Decode:       func(value string) (string, error) { return value, nil },
	}
	got, _, err := CallStructured(context.Background(), client, request, output)
	if err != nil || got != "ok" {
		t.Fatalf("CallStructured = (%q, %v)", got, err)
	}
}

func responseWithText(t *testing.T, text string) *chat.Response {
	t.Helper()
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	response, err := chat.NewResponse(chat.Choice{
		Index:        0,
		Message:      &message,
		FinishReason: chat.FinishReasonStop,
	})
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func mediaForStructuredTest() (*media.Media, error) {
	return media.NewBytes("image/png", []byte{1})
}
