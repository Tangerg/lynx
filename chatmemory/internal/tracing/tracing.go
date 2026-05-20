// Package tracing centralises the OTel span emission shared by every
// chat-memory provider in this module. Each provider's Read / Write /
// Clear entry point wraps its inner SDK work with the helpers here so
// the DB semconv attribute set stays consistent across backends.
//
// Per doc/OBSERVABILITY.md §3.5 the attributes follow the OTel DB
// semconv plus the lynx chat-memory specific extensions:
//
//	db.system                   — provider id ("postgres", "redis", ...)
//	db.operation.name           — "read" / "write" / "clear"
//	lynx.chat_memory.conv_id    — conversation id
//	lynx.chat_memory.msg_count  — number of messages (read result / write input)
package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	attrDBSystem        = "db.system"
	attrDBOperationName = "db.operation.name"
	attrLynxConvID      = "lynx.chat_memory.conv_id"
	attrLynxMsgCount    = "lynx.chat_memory.msg_count"
)

// tracerFor returns the per-provider tracer. Names follow
// `lynx/chatmemory/<system>` so backends that bucket by tracer name
// can distinguish providers without parsing attributes.
func tracerFor(system string) trace.Tracer {
	return otel.Tracer("lynx/chatmemory/" + system)
}

func start(ctx context.Context, system, op, convID string) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String(attrDBSystem, system),
		attribute.String(attrDBOperationName, op),
	}
	if convID != "" {
		attrs = append(attrs, attribute.String(attrLynxConvID, convID))
	}
	return tracerFor(system).Start(ctx, "chat.memory."+op+" "+system,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
}

// StartRead opens a span for [memory.Store.Read].
func StartRead(ctx context.Context, system, convID string) (context.Context, trace.Span) {
	return start(ctx, system, "read", convID)
}

// StartWrite opens a span for [memory.Store.Write]. msgCount records
// the number of messages being appended.
func StartWrite(ctx context.Context, system, convID string, msgCount int) (context.Context, trace.Span) {
	ctx, span := start(ctx, system, "write", convID)
	if msgCount > 0 {
		span.SetAttributes(attribute.Int(attrLynxMsgCount, msgCount))
	}
	return ctx, span
}

// StartClear opens a span for [memory.Store.Clear].
func StartClear(ctx context.Context, system, convID string) (context.Context, trace.Span) {
	return start(ctx, system, "clear", convID)
}

// Finish records err on span and ends it.
func Finish(span trace.Span, err error, extra ...attribute.KeyValue) {
	if len(extra) > 0 {
		span.SetAttributes(extra...)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// RecordReadResult stamps the resulting message count onto a Read span
// before ending it.
func RecordReadResult(span trace.Span, err error, msgCount int) {
	Finish(span, err, attribute.Int(attrLynxMsgCount, msgCount))
}
