package slog_test

import (
	"context"
	"errors"
	stdslog "log/slog"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/Tangerg/lynx/otel/slog"
)

// captureHandler records every slog.Record passed to it, for test assertions.
type captureHandler struct {
	mu      sync.Mutex
	records []stdslog.Record
}

func (h *captureHandler) Enabled(context.Context, stdslog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r stdslog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}
func (h *captureHandler) WithAttrs([]stdslog.Attr) stdslog.Handler { return h }
func (h *captureHandler) WithGroup(string) stdslog.Handler         { return h }

func (h *captureHandler) Records() []stdslog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]stdslog.Record, len(h.records))
	copy(out, h.records)
	return out
}

// attrMap extracts all attrs from a record into a map for easy assertion.
func attrMap(r stdslog.Record) map[string]any {
	m := make(map[string]any, r.NumAttrs())
	r.Attrs(func(a stdslog.Attr) bool {
		m[a.Key] = a.Value.Any()
		return true
	})
	return m
}

func newTestProvider(exporter sdktrace.SpanExporter) *sdktrace.TracerProvider {
	return sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
}

func TestExporter_SuccessSpan(t *testing.T) {
	handler := &captureHandler{}
	logger := stdslog.New(handler)
	exp := slog.NewExporter(logger)

	tp := newTestProvider(exp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "unit.test.op")
	span.SetAttributes(
		attribute.String("gen_ai.system", "openai"),
		attribute.Int("gen_ai.request.max_tokens", 1024),
	)
	span.AddEvent("first_token_received")
	span.End()

	records := handler.Records()
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}

	r := records[0]
	if r.Level != stdslog.LevelInfo {
		t.Errorf("want Info level, got %v", r.Level)
	}
	if r.Message != "span" {
		t.Errorf("want message %q, got %q", "span", r.Message)
	}

	attrs := attrMap(r)
	if attrs["name"] != "unit.test.op" {
		t.Errorf("want name=unit.test.op, got %v", attrs["name"])
	}
	if attrs["gen_ai.system"] != "openai" {
		t.Errorf("want gen_ai.system=openai, got %v", attrs["gen_ai.system"])
	}
	if attrs["gen_ai.request.max_tokens"] != int64(1024) {
		t.Errorf("want gen_ai.request.max_tokens=1024, got %v", attrs["gen_ai.request.max_tokens"])
	}
	if _, ok := attrs["trace_id"]; !ok {
		t.Error("trace_id attribute missing")
	}
	if _, ok := attrs["span_id"]; !ok {
		t.Error("span_id attribute missing")
	}
	if _, ok := attrs["duration"]; !ok {
		t.Error("duration attribute missing")
	}
	if events, ok := attrs["events"].([]string); !ok || len(events) != 1 || events[0] != "first_token_received" {
		t.Errorf("events mismatch: %v", attrs["events"])
	}
}

func TestExporter_ErrorSpan(t *testing.T) {
	handler := &captureHandler{}
	exp := slog.NewExporter(stdslog.New(handler))

	tp := newTestProvider(exp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	_, span := tp.Tracer("test").Start(context.Background(), "failing.op")
	span.RecordError(errors.New("boom"))
	span.SetStatus(codes.Error, "boom")
	span.End()

	records := handler.Records()
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}
	r := records[0]
	if r.Level != stdslog.LevelError {
		t.Errorf("want Error level, got %v", r.Level)
	}
	if r.Message != "span (error): boom" {
		t.Errorf("unexpected message: %q", r.Message)
	}
}

func TestExporter_ChildSpan_RecordsParent(t *testing.T) {
	handler := &captureHandler{}
	exp := slog.NewExporter(stdslog.New(handler))

	tp := newTestProvider(exp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tracer := tp.Tracer("test")
	ctx, parent := tracer.Start(context.Background(), "parent")
	_, child := tracer.Start(ctx, "child")
	child.End()
	parent.End()

	records := handler.Records()
	if len(records) != 2 {
		t.Fatalf("want 2 records, got %d", len(records))
	}

	// OTel exports finished spans in end-order, so child is first
	childAttrs := attrMap(records[0])
	parentAttrs := attrMap(records[1])

	parentSpanID, _ := parentAttrs["span_id"].(string)
	childParentID, _ := childAttrs["parent_span_id"].(string)
	if parentSpanID == "" || childParentID != parentSpanID {
		t.Errorf("child.parent_span_id=%q, want to equal parent.span_id=%q", childParentID, parentSpanID)
	}

	// root span has no parent_span_id
	if _, has := parentAttrs["parent_span_id"]; has {
		t.Error("root span should not have parent_span_id")
	}
}

func TestExporter_NilLogger_UsesDefault(t *testing.T) {
	// We don't assert on slog.Default()'s output, only that construction
	// does not panic and export runs without error.
	exp := slog.NewExporter(nil)

	tp := newTestProvider(exp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	_, span := tp.Tracer("test").Start(context.Background(), "smoke")
	span.End()
}

func TestExporter_Shutdown_ReturnsNil(t *testing.T) {
	exp := slog.NewExporter(stdslog.Default())
	if err := exp.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}
