package chatclient

import (
	"context"
	"errors"
	"iter"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
)

type callOnly struct {
	call func(context.Context, *chat.Request) (*chat.Response, error)
}

func (m callOnly) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	return m.call(ctx, request)
}

type streamOnly struct {
	stream func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error]
}

func (s streamOnly) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
	return s.stream(ctx, request)
}

type callAndStream struct {
	callOnly
	streamOnly
}

func TestNewRejectsInvalidConstruction(t *testing.T) {
	model := callOnly{call: successfulCall}
	negative := int64(-1)

	tests := []struct {
		name    string
		model   chat.Model
		options []Option
		want    error
	}{
		{name: "nil model", want: ErrNilModel},
		{name: "nil option", model: model, options: []Option{nil}},
		{name: "invalid defaults", model: model, options: []Option{WithDefaults(chat.Options{MaxTokens: &negative})}, want: chat.ErrInvalidOptions},
		{name: "nil explicit streamer", model: model, options: []Option{WithStreamer(nil)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := New(test.model, test.options...)
			if err == nil || client != nil {
				t.Fatalf("New() = (%v, %v), want nil client and error", client, err)
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("New() error = %v, want errors.Is(_, %v)", err, test.want)
			}
		})
	}
}

func TestCallMergesDefaultsAndProtectsCallerRequest(t *testing.T) {
	inline, err := media.NewBytes("image/png", []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if err := metadata.Set(inline.Metadata, "origin", "caller"); err != nil {
		t.Fatal(err)
	}
	message := chat.NewUserMessage(chat.NewMediaPart(inline))
	message.Metadata = metadata.New()
	if err := metadata.Set(message.Metadata, "turn", 1); err != nil {
		t.Fatal(err)
	}
	assistant := chat.NewAssistantMessage(
		chat.NewReasoningPart("thinking", []byte{4, 5}),
		chat.NewToolCallPart(chat.ToolCall{ID: "call-1", Name: "weather", Arguments: `{}`}),
	)
	toolMessage := chat.NewToolMessage(chat.ToolResult{ID: "call-1", Name: "weather", Result: "sunny"})

	requestMaxTokens := int64(7)
	request := &chat.Request{
		Messages: []chat.Message{message, assistant, toolMessage},
		Tools: []chat.ToolDefinition{{
			Name:        "weather",
			InputSchema: []byte(`{"type":"object"}`),
		}},
		Options: chat.Options{
			Model:     "request-model",
			MaxTokens: &requestMaxTokens,
			Stop:      []string{},
		},
		Extensions: metadata.New(),
	}
	if err := metadata.Set(request.Extensions, "test/value", "caller"); err != nil {
		t.Fatal(err)
	}

	temperature := 0.25
	topP := 0.8
	defaultMaxTokens := int64(99)
	defaults := chat.Options{
		Model:       "default-model",
		MaxTokens:   &defaultMaxTokens,
		Stop:        []string{"END"},
		Temperature: &temperature,
		TopP:        &topP,
	}
	defaultOption := WithDefaults(defaults)
	// WithDefaults snapshots at option construction, not when New is called.
	temperature = 1.5
	defaults.Stop[0] = "MUTATED"

	model := callOnly{call: func(_ context.Context, received *chat.Request) (*chat.Response, error) {
		if received == request {
			t.Fatal("model received caller-owned request pointer")
		}
		if received.Options.Model != "request-model" {
			t.Fatalf("model = %q, want request-model", received.Options.Model)
		}
		if received.Options.MaxTokens == nil || *received.Options.MaxTokens != 7 {
			t.Fatalf("max tokens = %v, want 7", received.Options.MaxTokens)
		}
		if received.Options.Temperature == nil || *received.Options.Temperature != 0.25 {
			t.Fatalf("temperature = %v, want snapshotted 0.25", received.Options.Temperature)
		}
		if received.Options.TopP == nil || *received.Options.TopP != 0.8 {
			t.Fatalf("top_p = %v, want inherited 0.8", received.Options.TopP)
		}
		if received.Options.Stop == nil || len(received.Options.Stop) != 0 {
			t.Fatalf("stop = %#v, want explicit non-nil empty override", received.Options.Stop)
		}

		// Mutate every reference-shaped request field. None may alias request.
		received.Messages[0].Metadata["turn"][0] = '9'
		received.Messages[0].Parts[0].Media.Source.Bytes[0] = 9
		received.Messages[0].Parts[0].Media.Metadata["origin"][1] = 'X'
		received.Messages[1].Parts[0].Signature[0] = 9
		received.Messages[1].Parts[1].ToolCall.Name = "mutated"
		received.Messages[2].Parts[0].ToolResult.Result = "mutated"
		received.Tools[0].InputSchema[2] = 'X'
		received.Extensions["test/value"][1] = 'X'
		*received.Options.MaxTokens = 8
		*received.Options.Temperature = 2

		return &chat.Response{ID: "response-1"}, nil
	}}
	client, err := New(model, defaultOption)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Call(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.ID != "response-1" {
		t.Fatalf("response ID = %q", response.ID)
	}

	if got := request.Messages[0].Metadata["turn"]; string(got) != "1" {
		t.Fatalf("caller message metadata mutated: %s", got)
	}
	if got := request.Messages[0].Parts[0].Media.Source.Bytes; !reflect.DeepEqual(got, []byte{1, 2, 3}) {
		t.Fatalf("caller media bytes mutated: %v", got)
	}
	if got := request.Messages[0].Parts[0].Media.Metadata["origin"]; string(got) != `"caller"` {
		t.Fatalf("caller media metadata mutated: %s", got)
	}
	if got := request.Messages[1].Parts[0].Signature; !reflect.DeepEqual(got, []byte{4, 5}) {
		t.Fatalf("caller reasoning signature mutated: %v", got)
	}
	if got := request.Messages[1].Parts[1].ToolCall.Name; got != "weather" {
		t.Fatalf("caller tool call mutated: %s", got)
	}
	if got := request.Messages[2].Parts[0].ToolResult.Result; got != "sunny" {
		t.Fatalf("caller tool result mutated: %s", got)
	}
	if got := string(request.Tools[0].InputSchema); got != `{"type":"object"}` {
		t.Fatalf("caller schema mutated: %s", got)
	}
	if got := request.Extensions["test/value"]; string(got) != `"caller"` {
		t.Fatalf("caller extension mutated: %s", got)
	}
	if requestMaxTokens != 7 {
		t.Fatalf("caller max tokens mutated: %d", requestMaxTokens)
	}
}

func TestCallRejectsInvalidRequestBeforeModel(t *testing.T) {
	var calls atomic.Int64
	client, err := New(callOnly{call: func(context.Context, *chat.Request) (*chat.Response, error) {
		calls.Add(1)
		return nil, nil
	}})
	if err != nil {
		t.Fatal(err)
	}

	response, err := client.Call(context.Background(), &chat.Request{})
	if response != nil || !errors.Is(err, chat.ErrInvalidRequest) {
		t.Fatalf("Call() = (%v, %v), want nil and ErrInvalidRequest", response, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("model called %d times for invalid request", calls.Load())
	}
}

func TestClientForwardsContextCancellationAndErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	model := callAndStream{
		callOnly: callOnly{call: func(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
			return nil, ctx.Err()
		}},
		streamOnly: streamOnly{stream: func(ctx context.Context, _ *chat.Request) iter.Seq2[*chat.Response, error] {
			return errorSequence(ctx.Err())
		}},
	}
	client, err := New(model)
	if err != nil {
		t.Fatal(err)
	}

	response, err := client.Call(ctx, textRequest("hello"))
	if response != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Call() = (%v, %v), want context.Canceled", response, err)
	}
	count := 0
	for streamResponse, streamErr := range client.Stream(ctx, textRequest("hello")) {
		count++
		if streamResponse != nil || !errors.Is(streamErr, context.Canceled) {
			t.Fatalf("stream yield = (%v, %v), want context.Canceled", streamResponse, streamErr)
		}
	}
	if count != 1 {
		t.Fatalf("stream yield count = %d, want 1", count)
	}
}

func TestMergeOptionsUsesEveryExplicitOverride(t *testing.T) {
	defaults := chat.Options{
		Model:            "default",
		FrequencyPenalty: pointer(0.1),
		MaxTokens:        pointer(int64(1)),
		PresencePenalty:  pointer(0.2),
		Stop:             []string{"default"},
		Temperature:      pointer(0.3),
		TopK:             pointer(int64(2)),
		TopP:             pointer(0.4),
	}
	overrides := chat.Options{
		Model:            "override",
		FrequencyPenalty: pointer(1.1),
		MaxTokens:        pointer(int64(10)),
		PresencePenalty:  pointer(1.2),
		Stop:             []string{"override"},
		Temperature:      pointer(1.3),
		TopK:             pointer(int64(20)),
		TopP:             pointer(0.9),
	}

	merged := mergeOptions(defaults, overrides)
	if !reflect.DeepEqual(merged, overrides) {
		t.Fatalf("merged = %#v, want %#v", merged, overrides)
	}
	*merged.FrequencyPenalty = 0
	*merged.MaxTokens = 99
	*merged.PresencePenalty = 0
	merged.Stop[0] = "mutated"
	*merged.Temperature = 0
	*merged.TopK = 99
	*merged.TopP = 0
	if *overrides.FrequencyPenalty != 1.1 || *overrides.MaxTokens != 10 ||
		*overrides.PresencePenalty != 1.2 || overrides.Stop[0] != "override" ||
		*overrides.Temperature != 1.3 || *overrides.TopK != 20 || *overrides.TopP != 0.9 {
		t.Fatalf("merged options alias overrides: %#v", overrides)
	}
}

func TestMergeOptionsKeepsDefaultsForUnspecifiedFields(t *testing.T) {
	defaults := chat.Options{
		Model:            "default",
		FrequencyPenalty: pointer(0.1),
		MaxTokens:        pointer(int64(1)),
		PresencePenalty:  pointer(0.2),
		Stop:             []string{"default"},
		Temperature:      pointer(0.3),
		TopK:             pointer(int64(2)),
		TopP:             pointer(0.4),
	}
	merged := mergeOptions(defaults, chat.Options{})
	if !reflect.DeepEqual(merged, defaults) {
		t.Fatalf("merged = %#v, want %#v", merged, defaults)
	}
	*merged.MaxTokens = 99
	merged.Stop[0] = "mutated"
	if *defaults.MaxTokens != 1 || defaults.Stop[0] != "default" {
		t.Fatalf("merged defaults alias input: %#v", defaults)
	}
}

func TestCallMiddlewareOrder(t *testing.T) {
	var events []string
	middleware := func(name string) chat.CallMiddleware {
		return func(next chat.Model) chat.Model {
			return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
				events = append(events, name+":before")
				response, err := next.Call(ctx, request)
				events = append(events, name+":after")
				return response, err
			})
		}
	}
	client, err := New(
		callOnly{call: func(context.Context, *chat.Request) (*chat.Response, error) {
			events = append(events, "model")
			return &chat.Response{}, nil
		}},
		WithCallMiddleware(middleware("outer"), nil),
		WithCallMiddleware(middleware("inner")),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(context.Background(), textRequest("hello")); err != nil {
		t.Fatal(err)
	}
	want := []string{"outer:before", "inner:before", "model", "inner:after", "outer:after"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestStreamAutoDiscoversCapabilitySnapshotsRequestAndReleasesOnStop(t *testing.T) {
	released := make(chan struct{})
	var seenText string
	model := callAndStream{
		callOnly: callOnly{call: successfulCall},
		streamOnly: streamOnly{stream: func(_ context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
			return func(yield func(*chat.Response, error) bool) {
				defer close(released)
				seenText = request.Messages[0].Text()
				if !yield(&chat.Response{ID: "first"}, nil) {
					return
				}
				yield(&chat.Response{ID: "second"}, nil)
			}
		}},
	}
	client, err := New(model)
	if err != nil {
		t.Fatal(err)
	}
	request := textRequest("before")
	sequence := client.Stream(context.Background(), request)
	request.Messages[0].Parts[0].Text = "after"

	count := 0
	for response, streamErr := range sequence {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		if response.ID != "first" {
			t.Fatalf("response ID = %q", response.ID)
		}
		count++
		break
	}
	if count != 1 || seenText != "before" {
		t.Fatalf("count/text = %d/%q, want 1/before", count, seenText)
	}
	select {
	case <-released:
	default:
		t.Fatal("stream resources were not synchronously released")
	}
}

func TestWithStreamerOverridesModelCapability(t *testing.T) {
	modelStreamCalled := false
	model := callAndStream{
		callOnly: callOnly{call: successfulCall},
		streamOnly: streamOnly{stream: func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
			modelStreamCalled = true
			return oneResponse("model")
		}},
	}
	explicit := streamOnly{stream: func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
		return oneResponse("explicit")
	}}
	client, err := New(model, WithStreamer(explicit))
	if err != nil {
		t.Fatal(err)
	}

	var id string
	for response, streamErr := range client.Stream(context.Background(), textRequest("hello")) {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		id = response.ID
	}
	if id != "explicit" || modelStreamCalled {
		t.Fatalf("ID/model stream called = %q/%v, want explicit/false", id, modelStreamCalled)
	}
}

func TestStreamUnsupportedAndInvalidRequestYieldOneTerminalError(t *testing.T) {
	var calls atomic.Int64
	client, err := New(callOnly{call: func(context.Context, *chat.Request) (*chat.Response, error) {
		calls.Add(1)
		return &chat.Response{}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		request *chat.Request
		want    error
	}{
		{name: "unsupported", request: textRequest("hello"), want: ErrStreamingUnsupported},
		{name: "invalid", request: &chat.Request{}, want: chat.ErrInvalidRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			count := 0
			for response, streamErr := range client.Stream(context.Background(), test.request) {
				count++
				if response != nil || !errors.Is(streamErr, test.want) {
					t.Fatalf("yield = (%v, %v), want nil and %v", response, streamErr, test.want)
				}
			}
			if count != 1 {
				t.Fatalf("yield count = %d, want 1", count)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("call capability unexpectedly invoked %d times", calls.Load())
	}
}

func TestStreamMiddlewareOrder(t *testing.T) {
	var events []string
	middleware := func(name string) chat.StreamMiddleware {
		return func(next chat.Streamer) chat.Streamer {
			return chat.StreamerFunc(func(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
				return func(yield func(*chat.Response, error) bool) {
					events = append(events, name+":before")
					for response, err := range next.Stream(ctx, request) {
						if !yield(response, err) {
							return
						}
					}
					events = append(events, name+":after")
				}
			})
		}
	}
	streamer := streamOnly{stream: func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) {
			events = append(events, "streamer")
			yield(&chat.Response{}, nil)
		}
	}}
	client, err := New(
		callOnly{call: successfulCall},
		WithStreamer(streamer),
		WithStreamMiddleware(middleware("outer"), nil, middleware("inner")),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, streamErr := range client.Stream(context.Background(), textRequest("hello")) {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
	}
	want := []string{"outer:before", "inner:before", "streamer", "inner:after", "outer:after"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestClientConfigurationIsSafeForConcurrentCalls(t *testing.T) {
	var calls atomic.Int64
	client, err := New(
		callOnly{call: func(_ context.Context, request *chat.Request) (*chat.Response, error) {
			if request.Options.Temperature == nil || *request.Options.Temperature != 0.4 {
				return nil, errors.New("missing default temperature")
			}
			calls.Add(1)
			return &chat.Response{}, nil
		}},
		WithDefaults(chat.Options{Temperature: pointer(0.4)}),
	)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 50
	var wait sync.WaitGroup
	errorsFound := make(chan error, goroutines)
	for range goroutines {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, callErr := client.Call(context.Background(), textRequest("hello"))
			errorsFound <- callErr
		}()
	}
	wait.Wait()
	close(errorsFound)
	for callErr := range errorsFound {
		if callErr != nil {
			t.Fatal(callErr)
		}
	}
	if got := calls.Load(); got != goroutines {
		t.Fatalf("calls = %d, want %d", got, goroutines)
	}
}

func TestNilStreamSequenceBecomesTerminalError(t *testing.T) {
	client, err := New(
		callOnly{call: successfulCall},
		WithStreamer(streamOnly{stream: func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
			return nil
		}}),
	)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for response, streamErr := range client.Stream(context.Background(), textRequest("hello")) {
		count++
		if response != nil || !errors.Is(streamErr, errNilStreamSequence) {
			t.Fatalf("yield = (%v, %v)", response, streamErr)
		}
	}
	if count != 1 {
		t.Fatalf("yield count = %d, want 1", count)
	}
}

func successfulCall(context.Context, *chat.Request) (*chat.Response, error) {
	return &chat.Response{}, nil
}

func oneResponse(id string) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		yield(&chat.Response{ID: id}, nil)
	}
}

func textRequest(text string) *chat.Request {
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart(text)))
	if err != nil {
		panic(err)
	}
	return request
}

func pointer[T any](value T) *T {
	return &value
}
