package chat_test

import (
	"errors"
	"slices"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
)

// installTraceCapture lazily installs a global TracerProvider backed
// by an in-memory exporter — OTel's global state isn't safe to swap
// multiple times across the same process (see
// https://github.com/open-telemetry/opentelemetry-go/issues/1893),
// so the provider is set exactly once and the exporter is reset
// between tests via t.Cleanup.
var (
	traceExporter     *tracetest.InMemoryExporter
	traceProviderOnce sync.Once
)

func installTraceCapture(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	traceProviderOnce.Do(func() {
		traceExporter = tracetest.NewInMemoryExporter()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(traceExporter))
		otel.SetTracerProvider(tp)
	})
	traceExporter.Reset()
	t.Cleanup(func() { traceExporter.Reset() })
	return traceExporter
}

// attrMap collapses a span stub's attribute slice into a name→value
// map for ergonomic assertions.
func attrMap(stub tracetest.SpanStub) map[string]attribute.Value {
	out := make(map[string]attribute.Value, len(stub.Attributes))
	for _, kv := range stub.Attributes {
		out[string(kv.Key)] = kv.Value
	}
	return out
}

func TestChatTracing_CallEmitsGenAISpan(t *testing.T) {
	exp := installTraceCapture(t)

	m := newFakeChatModel(t)
	m.provider = "openai"
	m.respond = func(*chat.Request) (*chat.Response, error) {
		resp, _ := chat.NewResponse(
			&chat.Result{
				AssistantMessage: chat.NewAssistantMessage("hi back"),
				Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
			},
			&chat.ResponseMetadata{
				ID:    "resp_42",
				Model: "gpt-4o-mini-2025-01-01",
				Usage: &model.Usage{PromptTokens: 12, CompletionTokens: 34},
			},
		)
		return resp, nil
	}

	opts, _ := chat.NewOptions("gpt-4o-mini")
	temp := 0.7
	maxTok := int64(100)
	opts.Temperature = &temp
	opts.MaxTokens = &maxTok

	client, _ := chat.NewClient(m)
	_, err := client.Chat().WithOptions(opts).WithUserPrompt("hi").Call().Response(t.Context())
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.Name != "chat gpt-4o-mini" {
		t.Fatalf("span name = %q, want %q", s.Name, "chat gpt-4o-mini")
	}

	a := attrMap(s)
	checks := []struct {
		key  string
		want string
	}{
		{"gen_ai.system", "openai"},
		{"gen_ai.operation.name", "chat"},
		{"gen_ai.request.model", "gpt-4o-mini"},
		{"gen_ai.response.id", "resp_42"},
		{"gen_ai.response.model", "gpt-4o-mini-2025-01-01"},
	}
	for _, c := range checks {
		if got := a[c.key].AsString(); got != c.want {
			t.Fatalf("attr %q = %q, want %q", c.key, got, c.want)
		}
	}

	if got := a["gen_ai.request.temperature"].AsFloat64(); got != 0.7 {
		t.Fatalf("temperature = %v, want 0.7", got)
	}
	if got := a["gen_ai.request.max_tokens"].AsInt64(); got != 100 {
		t.Fatalf("max_tokens = %v, want 100", got)
	}
	if got := a["gen_ai.usage.input_tokens"].AsInt64(); got != 12 {
		t.Fatalf("input_tokens = %v, want 12", got)
	}
	if got := a["gen_ai.usage.output_tokens"].AsInt64(); got != 34 {
		t.Fatalf("output_tokens = %v, want 34", got)
	}

	finish := a["gen_ai.response.finish_reasons"].AsStringSlice()
	if !slices.Equal(finish, []string{"stop"}) {
		t.Fatalf("finish_reasons = %v, want [stop]", finish)
	}
}

func TestChatTracing_CallErrorSetsSpanError(t *testing.T) {
	exp := installTraceCapture(t)

	m := newFakeChatModel(t)
	wantErr := errors.New("upstream boom")
	m.respond = func(*chat.Request) (*chat.Response, error) { return nil, wantErr }

	client, _ := chat.NewClient(m)
	_, err := client.Chat().WithUserPrompt("hi").Call().Response(t.Context())
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want chain to %v", err, wantErr)
	}

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Status.Code != codes.Error {
		t.Fatalf("span status = %v, want Error", spans[0].Status.Code)
	}
	if len(spans[0].Events) == 0 {
		t.Fatal("expected RecordError event on the span")
	}
}

func TestChatTracing_StreamEmitsFirstTokenEvent(t *testing.T) {
	exp := installTraceCapture(t)

	m := newFakeChatModel(t)
	m.streamYield = []*chat.Response{
		responseWithText("hi"),
		responseWithText(" there"),
	}

	client, _ := chat.NewClient(m)
	count := 0
	for _, err := range client.Chat().WithUserPrompt("hi").Stream().Response(t.Context()) {
		if err != nil {
			t.Fatalf("stream err: %v", err)
		}
		count++
	}
	if count != 2 {
		t.Fatalf("yielded %d chunks, want 2", count)
	}

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	gotFirstToken := false
	for _, e := range spans[0].Events {
		if e.Name == "gen_ai.stream.first_token_received" {
			gotFirstToken = true
		}
	}
	if !gotFirstToken {
		t.Fatal("expected gen_ai.stream.first_token_received event on the stream span")
	}
}
