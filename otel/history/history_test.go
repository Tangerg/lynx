package history_test

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/history"
	historyotel "github.com/Tangerg/lynx/otel/history"
)

type historyRecorder struct {
	messages []chat.Message
	err      error
}

func (s *historyRecorder) Read(context.Context, history.ConversationID) ([]chat.Message, error) {
	return s.messages, s.err
}

func (s *historyRecorder) Write(_ context.Context, _ history.ConversationID, messages ...chat.Message) error {
	s.messages = messages
	return s.err
}

func (s *historyRecorder) Clear(context.Context, history.ConversationID) error {
	s.messages = nil
	return s.err
}

func (s *historyRecorder) Conversations(context.Context) ([]history.ConversationID, error) {
	return []history.ConversationID{"one", "two"}, s.err
}

func newHistoryMiddleware(t *testing.T) (historyotel.Middleware, *tracetest.SpanRecorder) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	middleware, err := historyotel.New(historyotel.Config{
		System:         "  PostgreSQL  ",
		TracerProvider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	return middleware, recorder
}

func TestNewRequiresSystemAndPreservesMissingCapabilities(t *testing.T) {
	if _, err := historyotel.New(historyotel.Config{}); !errors.Is(err, historyotel.ErrInvalidConfig) {
		t.Fatalf("NewHistory() error = %v", err)
	}
	middleware, _ := newHistoryMiddleware(t)
	var store *historyRecorder
	var lister history.Lister
	if middleware.Store(store) != nil || middleware.Conversations(lister) != nil {
		t.Fatal("history instrumentation synthesized a missing capability")
	}
}

func TestStorePreservesResultsAndRecordsOperations(t *testing.T) {
	middleware, spans := newHistoryMiddleware(t)
	wantErr := errors.New("storage failed")
	store := &historyRecorder{err: wantErr}
	wrapped := middleware.Store(store)
	message := chat.NewUserMessage(chat.NewTextPart("hello"))

	if err := wrapped.Write(t.Context(), "conversation", message); !errors.Is(err, wantErr) {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := wrapped.Read(t.Context(), "conversation"); !errors.Is(err, wantErr) {
		t.Fatalf("Read() error = %v", err)
	}
	if err := wrapped.Clear(t.Context(), "conversation"); !errors.Is(err, wantErr) {
		t.Fatalf("Clear() error = %v", err)
	}

	ended := spans.Ended()
	if len(ended) != 3 {
		t.Fatalf("ended spans = %d, want 3", len(ended))
	}
	for index, operation := range []string{"write", "read", "clear"} {
		span := ended[index]
		if span.Name() != "history."+operation || span.SpanKind() != trace.SpanKindClient || span.Status().Code != codes.Error {
			t.Fatalf("span %d name/kind/status = %q/%v/%v", index, span.Name(), span.SpanKind(), span.Status())
		}
		attrs := spanAttributes(t, span)
		assertStringAttr(t, attrs, "db.system.name", "postgresql")
		assertStringAttr(t, attrs, "chat_history.operation.name", operation)
		assertStringAttr(t, attrs, "gen_ai.conversation.id", "conversation")
	}
}

func TestListerRecordsResultCount(t *testing.T) {
	middleware, spans := newHistoryMiddleware(t)
	ids, err := middleware.Conversations(&historyRecorder{}).Conversations(t.Context())
	if err != nil || len(ids) != 2 {
		t.Fatalf("Conversations() = %v, %v", ids, err)
	}
	attrs := spanAttributes(t, spans.Ended()[0])
	if count := attrs["chat_history.conversation.count"].AsInt64(); count != 2 {
		t.Fatalf("conversation count = %d, want 2", count)
	}
}

func spanAttributes(t *testing.T, span sdktrace.ReadOnlySpan) map[string]attribute.Value {
	t.Helper()
	values := make(map[string]attribute.Value, len(span.Attributes()))
	for _, attr := range span.Attributes() {
		values[string(attr.Key)] = attr.Value
	}
	return values
}

func assertStringAttr(t *testing.T, attrs map[string]attribute.Value, key, want string) {
	t.Helper()
	if got := attrs[key].AsString(); got != want {
		t.Fatalf("attribute %s = %q, want %q", key, got, want)
	}
}
