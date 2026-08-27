package chatclient

import (
	"context"
	"errors"
	"iter"
	"reflect"
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

func TestGenerationCallAndStreamUseOneTypedOutput(t *testing.T) {
	want := recipe{Name: "tea", Steps: []string{"boil", "steep"}}
	var callFormat, streamFormat *chat.OutputFormat
	model := callAndStream{
		callOnly: callOnly{call: func(_ context.Context, request *chat.Request) (*chat.Response, error) {
			callFormat = request.Options.OutputFormat.Clone()
			return responseWithText(t, `{"name":"tea","steps":["boil","steep"]}`), nil
		}},
		streamOnly: streamOnly{stream: func(_ context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
			streamFormat = request.Options.OutputFormat.Clone()
			return func(yield func(*chat.Response, error) bool) {
				for _, chunk := range []string{"```json\n{\"name\":\"tea\",", "\"steps\":[\"boil\",\"steep\"]}\n```"} {
					if !yield(responseWithText(t, chunk), nil) {
						return
					}
				}
			}
		}},
	}
	client, err := New(model, Config{})
	if err != nil {
		t.Fatal(err)
	}
	request := textRequest("make tea")
	generation := client.Output(JSON[recipe]())

	called, err := generation.Call(t.Context(), request)
	if err != nil || !reflect.DeepEqual(called, want) {
		t.Fatalf("Call = (%#v, %v), want %#v", called, err, want)
	}
	streamed, err := generation.Stream(t.Context(), request)
	if err != nil || !reflect.DeepEqual(streamed, want) {
		t.Fatalf("Stream = (%#v, %v), want %#v", streamed, err, want)
	}
	for name, format := range map[string]*chat.OutputFormat{"call": callFormat, "stream": streamFormat} {
		if format == nil || format.Type != chat.OutputFormatJSON {
			t.Errorf("%s output format = %#v, want JSON", name, format)
		}
	}
	if request.Options.OutputFormat != nil {
		t.Fatal("typed generation mutated caller-owned request")
	}
}

func TestGenerationSnapshotsSchemaAndOverridesClientDefault(t *testing.T) {
	format, err := JSONSchema[recipe]("recipe")
	if err != nil {
		t.Fatal(err)
	}
	textDefault, err := chat.NewOutputFormat(chat.OutputFormatText)
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(callOnly{call: func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		got := request.Options.OutputFormat
		if got == nil || got.Type != chat.OutputFormatJSONSchema || got.Name != "recipe" || got.Schema[0] != '{' {
			t.Fatalf("output format = %#v", got)
		}
		got.Schema[0] = '['
		return responseWithText(t, `{"name":"tea","steps":[]}`), nil
	}}, Config{Defaults: chat.Options{OutputFormat: &textDefault}})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.Output(format).Call(t.Context(), textRequest("tea")); err != nil {
		t.Fatal(err)
	}
	if format.contract.Schema[0] != '{' {
		t.Fatal("provider mutation escaped the generation snapshot")
	}
}

func TestGenerationRejectsDuplicateOrInvalidConstruction(t *testing.T) {
	var calls int
	client, err := New(callOnly{call: func(context.Context, *chat.Request) (*chat.Response, error) {
		calls++
		return responseWithText(t, `{}`), nil
	}}, Config{})
	if err != nil {
		t.Fatal(err)
	}

	request := textRequest("hello")
	requestFormat, err := chat.NewOutputFormat(chat.OutputFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	request.Options.OutputFormat = &requestFormat
	if _, err := client.Output(JSON[recipe]()).Call(t.Context(), request); !errors.Is(err, ErrInvalidOutputFormat) {
		t.Fatalf("duplicate output format = %v, want ErrInvalidOutputFormat", err)
	}
	if calls != 0 {
		t.Fatalf("model called %d times for duplicate output format", calls)
	}

	if _, err := client.Output(OutputFormat[recipe]{}).Call(t.Context(), textRequest("hello")); !errors.Is(err, ErrInvalidOutputFormat) {
		t.Fatalf("zero output format = %v, want ErrInvalidOutputFormat", err)
	}
	var nilClient *Client
	if _, err := nilClient.Output(JSON[recipe]()).Call(t.Context(), textRequest("hello")); !errors.Is(err, ErrNilClient) {
		t.Fatalf("nil client = %v, want ErrNilClient", err)
	}
}

func TestGenerationPreservesTransportErrors(t *testing.T) {
	upstream := errors.New("upstream")
	client, err := New(callOnly{call: func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, upstream
	}}, Config{})
	if err != nil {
		t.Fatal(err)
	}
	generation := client.Output(JSON[recipe]())
	if _, err := generation.Call(t.Context(), textRequest("hello")); !errors.Is(err, upstream) {
		t.Fatalf("Call error = %v, want upstream", err)
	}
	if _, err := generation.Stream(t.Context(), textRequest("hello")); !errors.Is(err, ErrStreamingUnsupported) {
		t.Fatalf("Stream error = %v, want ErrStreamingUnsupported", err)
	}
}
