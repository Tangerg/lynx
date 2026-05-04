package log_test

import (
	"bytes"
	"context"
	"errors"
	stdlog "log"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/Tangerg/lynx/observation/log"
)

// safeBuffer wraps bytes.Buffer with a mutex so concurrent log writes don't race.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func newTestLogger() (*stdlog.Logger, *safeBuffer) {
	buf := &safeBuffer{}
	// No prefix, no flags → deterministic output for assertions.
	return stdlog.New(buf, "", 0), buf
}

func newTestProvider(exporter sdktrace.SpanExporter) *sdktrace.TracerProvider {
	return sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
}

// linesOf splits the accumulated buffer into trimmed, non-empty lines.
func linesOf(buf *safeBuffer) []string {
	raw := strings.Split(buf.String(), "\n")
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func TestExporter_SuccessSpan(t *testing.T) {
	logger, buf := newTestLogger()
	exp := log.NewExporter(logger)

	tp := newTestProvider(exp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	_, span := tp.Tracer("test").Start(context.Background(), "unit.test.op")
	span.SetAttributes(
		attribute.String("gen_ai.system", "openai"),
		attribute.Int("gen_ai.request.max_tokens", 1024),
	)
	span.AddEvent("first_token_received")
	span.End()

	lines := linesOf(buf)
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %d: %q", len(lines), buf.String())
	}
	line := lines[0]

	requiredSubstrings := []string{
		`span `,                          // level marker (not error)
		`name="unit.test.op"`,            // quoted name
		`gen_ai.system=openai`,           // attribute
		`gen_ai.request.max_tokens=1024`, // int attribute
		`events=[first_token_received]`,  // events
		`trace_id=`,                      // has trace id
		`span_id=`,                       // has span id
		`duration=`,                      // has duration
	}
	for _, sub := range requiredSubstrings {
		if !strings.Contains(line, sub) {
			t.Errorf("line missing %q; got: %q", sub, line)
		}
	}
}

func TestExporter_ErrorSpan(t *testing.T) {
	logger, buf := newTestLogger()
	exp := log.NewExporter(logger)

	tp := newTestProvider(exp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	_, span := tp.Tracer("test").Start(context.Background(), "failing.op")
	span.RecordError(errors.New("boom"))
	span.SetStatus(codes.Error, "boom")
	span.End()

	lines := linesOf(buf)
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %d", len(lines))
	}
	line := lines[0]

	if !strings.HasPrefix(line, "[ERROR] span (error): boom ") {
		t.Errorf("unexpected error line: %q", line)
	}
}

func TestExporter_ErrorSpan_EmptyDescription(t *testing.T) {
	logger, buf := newTestLogger()
	exp := log.NewExporter(logger)

	tp := newTestProvider(exp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	_, span := tp.Tracer("test").Start(context.Background(), "failing.op")
	span.SetStatus(codes.Error, "")
	span.End()

	lines := linesOf(buf)
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "[ERROR] span (error) ") {
		t.Errorf("unexpected error line: %q", lines[0])
	}
}

func TestExporter_ChildSpan_RecordsParent(t *testing.T) {
	logger, buf := newTestLogger()
	exp := log.NewExporter(logger)

	tp := newTestProvider(exp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tracer := tp.Tracer("test")
	ctx, parent := tracer.Start(context.Background(), "parent")
	_, child := tracer.Start(ctx, "child")
	child.End()
	parent.End()

	lines := linesOf(buf)
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}

	// OTel exports finished spans in end order; child is first.
	childLine, parentLine := lines[0], lines[1]

	if !strings.Contains(childLine, "parent_span_id=") {
		t.Errorf("child line missing parent_span_id: %q", childLine)
	}
	if strings.Contains(parentLine, "parent_span_id=") {
		t.Errorf("root line should not have parent_span_id: %q", parentLine)
	}
}

func TestExporter_NilLogger_UsesDefault(t *testing.T) {
	// Construction must not panic; Export must not error.
	// (We can't easily assert on stdlog.Default()'s output here.)
	exp := log.NewExporter(nil)

	tp := newTestProvider(exp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	_, span := tp.Tracer("test").Start(context.Background(), "smoke")
	span.End()
}

func TestExporter_Shutdown_ReturnsNil(t *testing.T) {
	exp := log.NewExporter(stdlog.Default())
	if err := exp.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}
