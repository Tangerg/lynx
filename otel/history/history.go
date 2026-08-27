// Package history instruments conversation-history capabilities with OpenTelemetry.
package history

import (
	"context"
	"errors"
	"fmt"
	"strings"

	apiotel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/samber/lo"

	"github.com/Tangerg/scope/core/chat"
	corehistory "github.com/Tangerg/scope/core/history"
)

const (
	instrumentationName        = "github.com/Tangerg/scope/otel/history"
	operationAttributeName     = "chat_history.operation.name"
	messageCountAttribute      = "chat_history.message.count"
	conversationCountAttribute = "chat_history.conversation.count"
)

type historyOperation string

const (
	operationRead    historyOperation = "read"
	operationWrite   historyOperation = "write"
	operationClear   historyOperation = "clear"
	operationList    historyOperation = "list"
	operationReplace historyOperation = "replace"
	operationCount   historyOperation = "count"
)

func (o historyOperation) spanName() string { return "history." + string(o) }

var ErrInvalidConfig = errors.New("otel/history: invalid config")

// MiddlewareConfig identifies the storage system observed by history
// instrumentation.
type MiddlewareConfig struct {
	System         string
	TracerProvider trace.TracerProvider
}

func (c MiddlewareConfig) Validate() error {
	if strings.TrimSpace(c.System) == "" {
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

func NewMiddleware(config MiddlewareConfig) (Middleware, error) {
	if err := config.Validate(); err != nil {
		return Middleware{}, err
	}
	system := strings.ToLower(strings.TrimSpace(config.System))
	tracerProvider := config.TracerProvider
	if lo.IsNil(tracerProvider) {
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

// Store instruments the ordinary read, write, and clear capabilities.
func (m Middleware) Store(next corehistory.Store) corehistory.Store {
	if lo.IsNil(next) {
		return nil
	}
	return historyStore{middleware: m, next: next}
}

// Conversations instruments the optional cross-conversation listing
// capability without synthesizing it for stores that do not provide it.
func (m Middleware) Conversations(next corehistory.Lister) corehistory.Lister {
	if lo.IsNil(next) {
		return nil
	}
	return historyLister{middleware: m, next: next}
}

// Replace instruments the optional atomic replacement capability.
func (m Middleware) Replace(next corehistory.Replacer) corehistory.Replacer {
	if lo.IsNil(next) {
		return nil
	}
	return historyReplacer{middleware: m, next: next}
}

// Count instruments the optional message-count capability.
func (m Middleware) Count(next corehistory.Counter) corehistory.Counter {
	if lo.IsNil(next) {
		return nil
	}
	return historyCounter{middleware: m, next: next}
}

func (m Middleware) start(
	ctx context.Context,
	operation historyOperation,
	conversationID corehistory.ConversationID,
	extra ...attribute.KeyValue,
) (context.Context, trace.Span) {
	attrs := make([]attribute.KeyValue, 0, 3+len(extra))
	attrs = append(attrs,
		semconv.DBSystemNameKey.String(m.system),
		attribute.String(operationAttributeName, string(operation)),
	)
	if conversationID != "" {
		attrs = append(attrs, semconv.GenAIConversationID(conversationID.String()))
	}
	attrs = append(attrs, extra...)
	return m.tracer.Start(ctx, operation.spanName(),
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

func (h historyStore) Read(ctx context.Context, conversationID corehistory.ConversationID) ([]chat.Message, error) {
	ctx, span := h.middleware.start(ctx, operationRead, conversationID)
	messages, err := h.next.Read(ctx, conversationID)
	span.SetAttributes(attribute.Int(messageCountAttribute, len(messages)))
	finishHistorySpan(span, err)
	return messages, err
}

func (h historyStore) Write(ctx context.Context, conversationID corehistory.ConversationID, messages ...chat.Message) error {
	ctx, span := h.middleware.start(ctx, operationWrite, conversationID,
		attribute.Int(messageCountAttribute, len(messages)),
	)
	err := h.next.Write(ctx, conversationID, messages...)
	finishHistorySpan(span, err)
	return err
}

func (h historyStore) Clear(ctx context.Context, conversationID corehistory.ConversationID) error {
	ctx, span := h.middleware.start(ctx, operationClear, conversationID)
	err := h.next.Clear(ctx, conversationID)
	finishHistorySpan(span, err)
	return err
}

type historyLister struct {
	middleware Middleware
	next       corehistory.Lister
}

func (h historyLister) Conversations(ctx context.Context) ([]corehistory.ConversationID, error) {
	ctx, span := h.middleware.start(ctx, operationList, "")
	ids, err := h.next.Conversations(ctx)
	span.SetAttributes(attribute.Int(conversationCountAttribute, len(ids)))
	finishHistorySpan(span, err)
	return ids, err
}

type historyReplacer struct {
	middleware Middleware
	next       corehistory.Replacer
}

func (h historyReplacer) Replace(ctx context.Context, conversationID corehistory.ConversationID, messages ...chat.Message) error {
	ctx, span := h.middleware.start(ctx, operationReplace, conversationID,
		attribute.Int(messageCountAttribute, len(messages)),
	)
	err := h.next.Replace(ctx, conversationID, messages...)
	finishHistorySpan(span, err)
	return err
}

type historyCounter struct {
	middleware Middleware
	next       corehistory.Counter
}

func (h historyCounter) Count(ctx context.Context, conversationID corehistory.ConversationID) (int, error) {
	ctx, span := h.middleware.start(ctx, operationCount, conversationID)
	count, err := h.next.Count(ctx, conversationID)
	span.SetAttributes(attribute.Int(messageCountAttribute, count))
	finishHistorySpan(span, err)
	return count, err
}
