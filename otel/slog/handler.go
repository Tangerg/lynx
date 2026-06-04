package slog

import (
	"context"
	stdslog "log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

// Handler is a slog.Handler middleware that stamps the active span's
// trace_id / span_id onto every log record, so logs correlate with the
// trace they were emitted under — the Logs leg of full-link tracing. It
// delegates formatting to an inner handler (text / JSON); pair it with
// [NewExporter] so spans and logs land in the same slog stream keyed by
// the same trace_id.
//
// Usage at process start:
//
//	stdslog.SetDefault(stdslog.New(slog.NewHandler(
//	    stdslog.NewTextHandler(os.Stderr, &stdslog.HandlerOptions{Level: lvl}),
//	)))
//
// then anywhere on a traced path: stdslog.InfoContext(ctx, "msg", ...).
type Handler struct {
	inner stdslog.Handler
}

// NewHandler wraps inner so every record carries trace_id / span_id when
// the context holds a valid span. A nil inner defaults to a text handler
// on stderr.
func NewHandler(inner stdslog.Handler) *Handler {
	if inner == nil {
		inner = stdslog.NewTextHandler(os.Stderr, nil)
	}
	return &Handler{inner: inner}
}

func (h *Handler) Enabled(ctx context.Context, level stdslog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle stamps the active span context (if any) onto the record before
// delegating. A record emitted outside any span passes through unchanged.
func (h *Handler) Handle(ctx context.Context, rec stdslog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		rec.AddAttrs(
			stdslog.String("trace_id", sc.TraceID().String()),
			stdslog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.inner.Handle(ctx, rec)
}

func (h *Handler) WithAttrs(attrs []stdslog.Attr) stdslog.Handler {
	return &Handler{inner: h.inner.WithAttrs(attrs)}
}

func (h *Handler) WithGroup(name string) stdslog.Handler {
	return &Handler{inner: h.inner.WithGroup(name)}
}
