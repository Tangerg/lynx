// Package history instruments conversation-history capabilities with OpenTelemetry.
package history

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

	"github.com/Tangerg/lynx/core/chat"
	corehistory "github.com/Tangerg/lynx/core/history"
)

const instrumentationName = "github.com/Tangerg/lynx/otel/history"

// ErrInvalidConfig reports a missing database-system identity.
var ErrInvalidConfig = errors.New("otel/history: invalid config")

// Config identifies the storage system observed by history
// instrumentation.
type Config struct {
	System         string
	TracerProvider trace.TracerProvider
}

// Validate verifies the database-system identity required by history
// instrumentation.
func (config Config) Validate() error {
	if strings.TrimSpace(config.System) == "" {
		return fmt.Errorf("%w: system is required", ErrInvalidConfig)
	}
	return nil
}

// Middleware instruments history capabilities without entering a
// provider package or changing the wrapped operation's result.
type Middleware struct {
	system string
	tracer trace.Tracer
}

// New constructs history instrumentation for a composition root.
func New(config Config) (Middleware, error) {
	if err := config.Validate(); err != nil {
		return Middleware{}, err
	}
	system := strings.ToLower(strings.TrimSpace(config.System))
	tracerProvider := config.TracerProvider
	if isNilCapability(tracerProvider) {
		tracerProvider = apiotel.GetTracerProvider()
	}
	return Middleware{
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
func (m Middleware) Store(next corehistory.Store) corehistory.Store {
	if isNilCapability(next) {
		return nil
	}
	return historyStore{middleware: m, next: next}
}

// Conversations instruments the optional cross-conversation listing
// capability without synthesizing it for stores that do not provide it.
func (m Middleware) Conversations(next corehistory.Lister) corehistory.Lister {
	if isNilCapability(next) {
		return nil
	}
	return historyLister{middleware: m, next: next}
}

// Replace instruments the optional atomic replacement capability.
func (m Middleware) Replace(next corehistory.Replacer) corehistory.Replacer {
	if isNilCapability(next) {
		return nil
	}
	return historyReplacer{middleware: m, next: next}
}

// Count instruments the optional message-count capability.
func (m Middleware) Count(next corehistory.Counter) corehistory.Counter {
	if isNilCapability(next) {
		return nil
	}
	return historyCounter{middleware: m, next: next}
}

func (m Middleware) start(
	ctx context.Context,
	operation string,
	conversationID corehistory.ConversationID,
	extra ...attribute.KeyValue,
) (context.Context, trace.Span) {
	attrs := make([]attribute.KeyValue, 0, 3+len(extra))
	attrs = append(attrs,
		semconv.DBSystemNameKey.String(m.system),
		attribute.String("chat_history.operation.name", operation),
	)
	if conversationID != "" {
		attrs = append(attrs, semconv.GenAIConversationID(conversationID.String()))
	}
	attrs = append(attrs, extra...)
	return m.tracer.Start(ctx, "history."+operation,
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
	middleware Middleware
	next       corehistory.Store
}

func (s historyStore) Read(ctx context.Context, conversationID corehistory.ConversationID) ([]chat.Message, error) {
	ctx, span := s.middleware.start(ctx, "read", conversationID)
	messages, err := s.next.Read(ctx, conversationID)
	span.SetAttributes(attribute.Int("chat_history.message.count", len(messages)))
	finishHistorySpan(span, err)
	return messages, err
}

func (s historyStore) Write(ctx context.Context, conversationID corehistory.ConversationID, messages ...chat.Message) error {
	ctx, span := s.middleware.start(ctx, "write", conversationID,
		attribute.Int("chat_history.message.count", len(messages)),
	)
	err := s.next.Write(ctx, conversationID, messages...)
	finishHistorySpan(span, err)
	return err
}

func (s historyStore) Clear(ctx context.Context, conversationID corehistory.ConversationID) error {
	ctx, span := s.middleware.start(ctx, "clear", conversationID)
	err := s.next.Clear(ctx, conversationID)
	finishHistorySpan(span, err)
	return err
}

type historyLister struct {
	middleware Middleware
	next       corehistory.Lister
}

func (l historyLister) Conversations(ctx context.Context) ([]corehistory.ConversationID, error) {
	ctx, span := l.middleware.start(ctx, "list", "")
	ids, err := l.next.Conversations(ctx)
	span.SetAttributes(attribute.Int("chat_history.conversation.count", len(ids)))
	finishHistorySpan(span, err)
	return ids, err
}

type historyReplacer struct {
	middleware Middleware
	next       corehistory.Replacer
}

func (r historyReplacer) Replace(ctx context.Context, conversationID corehistory.ConversationID, messages ...chat.Message) error {
	ctx, span := r.middleware.start(ctx, "replace", conversationID,
		attribute.Int("chat_history.message.count", len(messages)),
	)
	err := r.next.Replace(ctx, conversationID, messages...)
	finishHistorySpan(span, err)
	return err
}

type historyCounter struct {
	middleware Middleware
	next       corehistory.Counter
}

func (c historyCounter) Count(ctx context.Context, conversationID corehistory.ConversationID) (int, error) {
	ctx, span := c.middleware.start(ctx, "count", conversationID)
	count, err := c.next.Count(ctx, conversationID)
	span.SetAttributes(attribute.Int("chat_history.message.count", count))
	finishHistorySpan(span, err)
	return count, err
}
