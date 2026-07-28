package turn_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

var (
	turnTraceOnce     sync.Once
	turnTraceExporter *tracetest.InMemoryExporter
	turnTraceProvider *sdktrace.TracerProvider
)

func installTurnTraceCapture(t *testing.T) (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	t.Helper()
	turnTraceOnce.Do(func() {
		turnTraceExporter = tracetest.NewInMemoryExporter()
		turnTraceProvider = sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
			sdktrace.WithSyncer(turnTraceExporter),
		)
		otel.SetTracerProvider(turnTraceProvider)
	})
	turnTraceExporter.Reset()
	t.Cleanup(turnTraceExporter.Reset)
	return turnTraceProvider, turnTraceExporter
}

// turnSpan selects the span belonging to one turn.
//
// The exporter is process-global — the tracer provider has to be, because the
// code under test uses the global one — and a sibling test's drive or cleanup
// goroutine can still be ending its span after this test resets it. Asserting a
// total span count therefore measures other tests' timing, not this turn's
// behavior. Selecting by run id measures only what this turn recorded.
func turnSpan(spans tracetest.SpanStubs, turnID string) (tracetest.SpanStub, bool) {
	for _, span := range spans {
		for _, attr := range span.Attributes {
			if string(attr.Key) == "run.id" && attr.Value.AsString() == turnID {
				return span, true
			}
		}
	}
	return tracetest.SpanStub{}, false
}

func TestTerminalDiscardFailureIsRecordedUntilCleanupSucceeds(t *testing.T) {
	discardErr := errors.New("snapshot discard failed")
	_, exporter := installTurnTraceCapture(t)

	stub := &stubEngine{runReply: "ok", discardErr: discardErr}
	dispatcher := mustTurn(turn.New(turnDeps(stub)))
	handle, err := dispatcher.StartTurn(t.Context(), runs.StartTurn{SessionID: "s", Message: "hi"})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, err := dispatcher.Events(t.Context(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	for range events {
	}
	if err := dispatcher.Cancel(t.Context(), handle); err != nil &&
		!errors.Is(err, turn.ErrTurnNotFound) &&
		!errors.Is(err, discardErr) {
		t.Fatalf("join terminal cleanup: %v", err)
	}
	stub.lastProcess.Load().discardErr = nil
	if err := dispatcher.Cancel(t.Context(), handle); err != nil {
		t.Fatalf("retry terminal cleanup: %v", err)
	}
	if err := dispatcher.Cancel(t.Context(), handle); !errors.Is(err, turn.ErrTurnNotFound) {
		t.Fatalf("Cancel after joined terminal cleanup = %v, want ErrTurnNotFound", err)
	}

	span, ok := turnSpan(exporter.GetSpans(), handle.TurnID)
	if !ok {
		t.Fatalf("no turn span was recorded for turn %q", handle.TurnID)
	}
	for _, event := range span.Events {
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
	tp, _ := installTurnTraceCapture(t)

	// Open an entry span and start the turn under it — mirrors the HTTP
	// transport opening a server span before runs.start.
	entryCtx, entry := tp.Tracer("test/entry").Start(context.Background(), "entry")
	wantTrace := entry.SpanContext().TraceID()

	stub := &stubEngine{runReply: "ok"}
	dispatcher := mustTurn(turn.New(turnDeps(stub)))
	handle, err := dispatcher.StartTurn(entryCtx, runs.StartTurn{SessionID: "s", Message: "hi"})
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
