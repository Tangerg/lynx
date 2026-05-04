package log

import (
	"context"
	"fmt"
	stdlog "log"
	"strings"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Exporter writes finished OpenTelemetry spans to a stdlib *log.Logger.
//
// It implements sdktrace.SpanExporter and is intended for projects still on
// the stdlib log package. Each span becomes a single line in a logfmt-like
// format. For structured output (key=value attrs as fields), use the sibling
// slog exporter instead.
//
// Error spans are prefixed with "[ERROR] " and include the status
// description (when present) as the leading message.
type Exporter struct {
	logger *stdlog.Logger
}

// NewExporter returns a new stdlib log exporter.
// If logger is nil, stdlog.Default() is used.
func NewExporter(logger *stdlog.Logger) *Exporter {
	if logger == nil {
		logger = stdlog.Default()
	}
	return &Exporter{logger: logger}
}

// ExportSpans writes each provided span as a single log line.
func (e *Exporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	for _, span := range spans {
		e.logger.Print(formatSpan(span))
	}
	return nil
}

// Shutdown releases any resources held by the exporter.
// This implementation is stateless and always returns nil.
func (e *Exporter) Shutdown(_ context.Context) error { return nil }

// formatSpan renders a single span as a logfmt-like single line.
// Layout:
//
//	<level> <trace/span ids> name=<q> duration=<d> [parent_span_id=...] <k=v pairs> [events=[...]]
func formatSpan(span sdktrace.ReadOnlySpan) string {
	var b strings.Builder
	b.Grow(256)

	// Level marker
	status := span.Status()
	if status.Code == codes.Error {
		if status.Description != "" {
			b.WriteString("[ERROR] span (error): ")
			b.WriteString(status.Description)
		} else {
			b.WriteString("[ERROR] span (error)")
		}
		b.WriteByte(' ')
	} else {
		b.WriteString("span ")
	}

	// Core identifiers
	sc := span.SpanContext()
	fmt.Fprintf(&b, "trace_id=%s span_id=%s ", sc.TraceID(), sc.SpanID())
	if parent := span.Parent(); parent.HasSpanID() {
		fmt.Fprintf(&b, "parent_span_id=%s ", parent.SpanID())
	}
	fmt.Fprintf(&b, "name=%q duration=%s",
		span.Name(), span.EndTime().Sub(span.StartTime()))

	// Attributes (gen_ai.*, db.*, lynx.*, etc.)
	for _, kv := range span.Attributes() {
		b.WriteByte(' ')
		b.WriteString(string(kv.Key))
		b.WriteByte('=')
		b.WriteString(kv.Value.Emit())
	}

	// Event names
	if evs := span.Events(); len(evs) > 0 {
		b.WriteString(" events=[")
		for i, ev := range evs {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(ev.Name)
		}
		b.WriteByte(']')
	}

	return b.String()
}
