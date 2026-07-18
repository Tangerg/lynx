package turn_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
)

func TestTerminalDiscardFailureIsRecordedBeforeTurnSpanEnds(t *testing.T) {
	discardErr := errors.New("snapshot discard failed")
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		if err := tp.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Errorf("shutdown tracer provider: %v", err)
		}
	})

	stub := &stubEngine{runReply: "ok", discardErr: discardErr}
	dispatcher := mustTurn(turn.New(turnDeps(stub)))
	handle, err := dispatcher.StartTurn(t.Context(), turn.StartTurnRequest{SessionID: "s", Message: "hi"})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, err := dispatcher.Events(t.Context(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	for range events {
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	for _, event := range spans[0].Events {
		for _, attr := range event.Attributes {
			if event.Name == "exception" && string(attr.Key) == "exception.message" && strings.Contains(attr.Value.AsString(), discardErr.Error()) {
				return
			}
		}
	}
	t.Fatal("terminal discard failure was not recorded on the turn span")
}

// TestStartTurn_PropagatesEntryTrace is the full-link tracing guarantee:
// the turn's lifetime ctx (which the engine and every span below it runs
// under) is derived from the caller's ctx via context.WithoutCancel, so
// the engine work lands in the SAME trace as the entry span — not a fresh
// root. Before the WithoutCancel fix the turn ctx was context.Background-
// rooted and this trace id would differ (the regression this guards).
func TestStartTurn_PropagatesEntryTrace(t *testing.T) {
	// A real (SDK) provider so spans carry a valid, recorded SpanContext;
	// the global tracer otherwise compiles to a no-op with an invalid id.
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	// Open an entry span and start the turn under it — mirrors the HTTP
	// transport opening a server span before runs.start.
	entryCtx, entry := tp.Tracer("test/entry").Start(context.Background(), "entry")
	wantTrace := entry.SpanContext().TraceID()

	stub := &stubEngine{runReply: "ok"}
	dispatcher := mustTurn(turn.New(turnDeps(stub)))
	handle, err := dispatcher.StartTurn(entryCtx, turn.StartTurnRequest{SessionID: "s", Message: "hi"})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	// The caller's ctx ending must NOT kill the turn — cancel it right away
	// and confirm the turn still completes (the other half of WithoutCancel).
	entry.End()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, _ := dispatcher.Events(ctx, handle)
	for range events { // drain to TurnEnd
	}

	stub.mu.Lock()
	got := stub.lastCtx
	stub.mu.Unlock()
	if got == nil {
		t.Fatal("engine never ran (no captured ctx)")
	}
	gotTrace := trace.SpanContextFromContext(got).TraceID()
	if gotTrace != wantTrace {
		t.Errorf("engine ran under trace %s, want the entry trace %s (full-link broken)", gotTrace, wantTrace)
	}
}
