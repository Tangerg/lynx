package slog

import (
	"context"
	stdslog "log/slog"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Exporter writes finished OpenTelemetry spans to a log/slog logger.
//
// It implements sdktrace.SpanExporter and is intended to be installed on a
// TracerProvider via sdktrace.WithSyncer (for dev/debug, synchronous output)
// or sdktrace.WithBatcher (for production-ish batched output).
//
// Each span becomes a single slog record. The record message is "span" for
// OK/Unset status and "span (error): <description>" for Error status, with
// the log level promoted to Error accordingly.
//
// The following attributes are always included:
//
//   - trace_id       (span.SpanContext().TraceID())
//   - span_id        (span.SpanContext().SpanID())
//   - name           (span.Name())
//   - duration       (EndTime - StartTime)
//   - parent_span_id (only if the span has a parent)
//
// All span attributes and event names are attached as additional slog
// attributes, preserving their OTel key names (e.g. "gen_ai.system",
// "gen_ai.agent.name").
type Exporter struct {
	logger *stdslog.Logger
}

// NewExporter returns a new slog exporter.
// If logger is nil, stdslog.Default() is used.
func NewExporter(logger *stdslog.Logger) *Exporter {
	if logger == nil {
		logger = stdslog.Default()
	}
	return &Exporter{logger: logger}
}

// ExportSpans writes each provided span as a single slog record.
// Always returns nil; slog errors (if any) are ignored because they would
// only propagate into the tracer provider's error handler and typically
// indicate a misconfigured handler rather than a recoverable condition.
func (e *Exporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	for _, span := range spans {
		attrs := make([]stdslog.Attr, 0, 6+len(span.Attributes()))

		sc := span.SpanContext()
		attrs = append(attrs,
			stdslog.String("trace_id", sc.TraceID().String()),
			stdslog.String("span_id", sc.SpanID().String()),
			stdslog.String("name", span.Name()),
			stdslog.Duration("duration", span.EndTime().Sub(span.StartTime())),
		)

		if parent := span.Parent(); parent.HasSpanID() {
			attrs = append(attrs, stdslog.String("parent_span_id", parent.SpanID().String()))
		}

		for _, kv := range span.Attributes() {
			attrs = append(attrs, stdslog.Any(string(kv.Key), kv.Value.AsInterface()))
		}

		if evs := span.Events(); len(evs) > 0 {
			names := make([]string, len(evs))
			for i, ev := range evs {
				names[i] = ev.Name
			}
			attrs = append(attrs, stdslog.Any("events", names))
		}

		level := stdslog.LevelInfo
		msg := "span"
		if status := span.Status(); status.Code == codes.Error {
			level = stdslog.LevelError
			if status.Description != "" {
				msg = "span (error): " + status.Description
			} else {
				msg = "span (error)"
			}
		}

		e.logger.LogAttrs(ctx, level, msg, attrs...)
	}
	return nil
}

// Shutdown releases any resources held by the exporter.
// This implementation is stateless and always returns nil.
func (e *Exporter) Shutdown(ctx context.Context) error { return nil }
