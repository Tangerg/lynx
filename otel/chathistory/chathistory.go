// Package chathistory instruments chat-history capabilities with OpenTelemetry.
package chathistory

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	apiotel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"

	corehistory "github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
)

const instrumentationName = "github.com/Tangerg/lynx/otel/chathistory"

// ErrInvalidConfig reports a missing database-system identity.
var ErrInvalidConfig = errors.New("otel/chathistory: invalid config")

// Config identifies the storage system observed by history
// instrumentation.
type Config struct {
	System         string
	TracerProvider trace.TracerProvider
}

// Middleware instruments chathistory capabilities without entering a
// provider package or changing the wrapped operation's result.
type Middleware struct {
	system string
	tracer trace.Tracer
}

// New constructs history instrumentation for a composition root.
func New(config Config) (*Middleware, error) {
	system := strings.ToLower(strings.TrimSpace(config.System))
	if system == "" {
		return nil, fmt.Errorf("%w: system is required", ErrInvalidConfig)
	}
	tracerProvider := config.TracerProvider
	if isNilCapability(tracerProvider) {
		tracerProvider = apiotel.GetTracerProvider()
	}
	return &Middleware{
		system: system,
		tracer: tracerProvider.Tracer(
			instrumentationName,
			trace.WithSchemaURL(semconv.SchemaURL),
		),
	}, nil
}

func isNilCapability(value any) bool {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return true
	}
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// Store instruments the ordinary read, write, and clear capabilities.
func (m *Middleware) Store(next corehistory.Store) corehistory.Store {
	if isNilCapability(next) {
		return nil
	}
	return historyStore{
		read: func(ctx context.Context, conversationID string) ([]chat.Message, error) {
			ctx, span := m.start(ctx, "read", conversationID)
			messages, err := next.Read(ctx, conversationID)
			span.SetAttributes(attribute.Int("chat_history.message.count", len(messages)))
			finishHistorySpan(span, err)
			return messages, err
		},
		write: func(ctx context.Context, conversationID string, messages ...chat.Message) error {
			ctx, span := m.start(ctx, "write", conversationID,
				attribute.Int("chat_history.message.count", len(messages)),
			)
			err := next.Write(ctx, conversationID, messages...)
			finishHistorySpan(span, err)
			return err
		},
		clear: func(ctx context.Context, conversationID string) error {
			ctx, span := m.start(ctx, "clear", conversationID)
			err := next.Clear(ctx, conversationID)
			finishHistorySpan(span, err)
			return err
		},
	}
}

// Conversations instruments the optional cross-conversation listing
// capability without synthesizing it for stores that do not provide it.
func (m *Middleware) Conversations(next corehistory.Lister) corehistory.Lister {
	if isNilCapability(next) {
		return nil
	}
	return historyListerFunc(func(ctx context.Context) ([]string, error) {
		ctx, span := m.start(ctx, "list", "")
		ids, err := next.Conversations(ctx)
		span.SetAttributes(attribute.Int("chat_history.conversation.count", len(ids)))
		finishHistorySpan(span, err)
		return ids, err
	})
}

// Replace instruments the optional atomic replacement capability.
func (m *Middleware) Replace(next corehistory.Replacer) corehistory.Replacer {
	if isNilCapability(next) {
		return nil
	}
	return historyReplacerFunc(func(ctx context.Context, conversationID string, messages ...chat.Message) error {
		ctx, span := m.start(ctx, "replace", conversationID,
			attribute.Int("chat_history.message.count", len(messages)),
		)
		err := next.Replace(ctx, conversationID, messages...)
		finishHistorySpan(span, err)
		return err
	})
}

// Count instruments the optional message-count capability.
func (m *Middleware) Count(next corehistory.Counter) corehistory.Counter {
	if isNilCapability(next) {
		return nil
	}
	return historyCounterFunc(func(ctx context.Context, conversationID string) (int, error) {
		ctx, span := m.start(ctx, "count", conversationID)
		count, err := next.Count(ctx, conversationID)
		span.SetAttributes(attribute.Int("chat_history.message.count", count))
		finishHistorySpan(span, err)
		return count, err
	})
}

func (m *Middleware) start(
	ctx context.Context,
	operation string,
	conversationID string,
	extra ...attribute.KeyValue,
) (context.Context, trace.Span) {
	attrs := make([]attribute.KeyValue, 0, 3+len(extra))
	attrs = append(attrs,
		semconv.DBSystemNameKey.String(m.system),
		attribute.String("chat_history.operation.name", operation),
	)
	if conversationID != "" {
		attrs = append(attrs, semconv.GenAIConversationID(conversationID))
	}
	attrs = append(attrs, extra...)
	return m.tracer.Start(ctx, "chathistory."+operation,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
}

func finishHistorySpan(span trace.Span, err error) {
	defer span.End()
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

type historyStore struct {
	read  func(context.Context, string) ([]chat.Message, error)
	write func(context.Context, string, ...chat.Message) error
	clear func(context.Context, string) error
}

func (s historyStore) Read(ctx context.Context, conversationID string) ([]chat.Message, error) {
	return s.read(ctx, conversationID)
}

func (s historyStore) Write(ctx context.Context, conversationID string, messages ...chat.Message) error {
	return s.write(ctx, conversationID, messages...)
}

func (s historyStore) Clear(ctx context.Context, conversationID string) error {
	return s.clear(ctx, conversationID)
}

type historyListerFunc func(context.Context) ([]string, error)

func (f historyListerFunc) Conversations(ctx context.Context) ([]string, error) {
	return f(ctx)
}

type historyReplacerFunc func(context.Context, string, ...chat.Message) error

func (f historyReplacerFunc) Replace(ctx context.Context, conversationID string, messages ...chat.Message) error {
	return f(ctx, conversationID, messages...)
}

type historyCounterFunc func(context.Context, string) (int, error)

func (f historyCounterFunc) Count(ctx context.Context, conversationID string) (int, error) {
	return f(ctx, conversationID)
}
