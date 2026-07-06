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
	attrConvID          = "gen_ai.conversation.id"
	attrMsgCount        = "chat_history.msg_count"
	attrConvCount       = "chat_history.conv_count"
)

// tracerFor returns the per-provider tracer. Names follow
// `lynx/chathistory/<system>` so backends that bucket by tracer name
// can distinguish providers without parsing attributes.
func tracerFor(system string) trace.Tracer {
	return otel.Tracer("lynx/chathistory/" + system)
}

func start(ctx context.Context, system, op, convID string) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String(attrDBSystem, system),
		attribute.String(attrDBOperationName, op),
	}
	if convID != "" {
		attrs = append(attrs, attribute.String(attrConvID, convID))
	}
	return tracerFor(system).Start(ctx, "chat.history."+op+" "+system,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
}

// StartRead opens a span for [history.Store.Read].
func StartRead(ctx context.Context, system, convID string) (context.Context, trace.Span) {
	return start(ctx, system, "read", convID)
}

// StartWrite opens a span for [history.Store.Write]. msgCount records
// the number of messages being appended.
func StartWrite(ctx context.Context, system, convID string, msgCount int) (context.Context, trace.Span) {
	ctx, span := start(ctx, system, "write", convID)
	if msgCount > 0 {
		span.SetAttributes(attribute.Int(attrMsgCount, msgCount))
	}
	return ctx, span
}

// StartClear opens a span for [history.Store.Clear].
func StartClear(ctx context.Context, system, convID string) (context.Context, trace.Span) {
	return start(ctx, system, "clear", convID)
}

// StartList opens a span for [history.Lister.Conversations] — a
// deliberate cross-conversation scan, so it carries no convID attribute.
func StartList(ctx context.Context, system string) (context.Context, trace.Span) {
	return start(ctx, system, "list", "")
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
	Finish(span, err, attribute.Int(attrMsgCount, msgCount))
}

// RecordListResult stamps the number of conversations found onto a List
// span before ending it.
func RecordListResult(span trace.Span, err error, convCount int) {
	Finish(span, err, attribute.Int(attrConvCount, convCount))
}
