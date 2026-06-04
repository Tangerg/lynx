package slog_test

import (
	"context"
	stdslog "log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/otel/slog"
)

// TestHandler_StampsTraceContext verifies the context-aware handler adds
// trace_id / span_id when the context carries a valid span, and leaves a
// record untouched otherwise (the Logs leg of full-link tracing).
func TestHandler_StampsTraceContext(t *testing.T) {
	cap := &captureHandler{}
	logger := stdslog.New(slog.NewHandler(cap))

	traceID, _ := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	spanID, _ := trace.SpanIDFromHex("0102030405060708")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	logger.InfoContext(ctx, "traced")
	logger.InfoContext(context.Background(), "untraced")

	recs := cap.Records()
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}

	attrsOf := func(r stdslog.Record) map[string]string {
		m := map[string]string{}
		r.Attrs(func(a stdslog.Attr) bool {
			m[a.Key] = a.Value.String()
			return true
		})
		return m
	}

	traced := attrsOf(recs[0])
	if traced["trace_id"] != traceID.String() {
		t.Fatalf("traced trace_id = %q, want %q", traced["trace_id"], traceID.String())
	}
	if traced["span_id"] != spanID.String() {
		t.Fatalf("traced span_id = %q, want %q", traced["span_id"], spanID.String())
	}

	if untraced := attrsOf(recs[1]); untraced["trace_id"] != "" || untraced["span_id"] != "" {
		t.Fatalf("untraced record must carry no trace ids: %+v", untraced)
	}
}
