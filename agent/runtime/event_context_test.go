package runtime_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/runtime"
)

var (
	runtimeTraceExporter *tracetest.InMemoryExporter
	runtimeTraceOnce     sync.Once
)

func installRuntimeTraceCapture(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	runtimeTraceOnce.Do(func() {
		runtimeTraceExporter = tracetest.NewInMemoryExporter()
		provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(runtimeTraceExporter))
		otel.SetTracerProvider(provider)
	})
	runtimeTraceExporter.Reset()
	t.Cleanup(func() { runtimeTraceExporter.Reset() })
	return runtimeTraceExporter
}

type readyPanicListener struct {
	done atomic.Bool
}

func (*readyPanicListener) Name() string { return "ready-panic-listener" }

func (l *readyPanicListener) OnEvent(e event.Event) {
	if _, ok := e.(event.ReadyToPlan); ok && l.done.CompareAndSwap(false, true) {
		panic("ready listener failed")
	}
}

type customPanicListener struct {
	done atomic.Bool
}

func (*customPanicListener) Name() string { return "custom-panic-listener" }

func (l *customPanicListener) OnEvent(e event.Event) {
	ev, ok := e.(event.ReplanRequested)
	if ok && ev.Reason == "custom" && l.done.CompareAndSwap(false, true) {
		panic("custom listener failed")
	}
}

func TestRuntimeEventPanicSpanKeepsRunTrace(t *testing.T) {
	exp := installRuntimeTraceCapture(t)

	a := agent.New("event-trace").
		Actions(agent.NewAction("count",
			func(_ context.Context, _ *core.ProcessContext, in word) (wordCount, error) {
				return wordCount{Count: len(in.Text)}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[wordCount](core.Goal{Description: "counted"})).
		Build()

	platform := agent.NewPlatform(runtime.PlatformConfig{
		Extensions: []core.Extension{&readyPanicListener{}},
	})
	mustDeploy(t, platform, a)

	ctx, parent := otel.Tracer("test/runtime").Start(context.Background(), "test-parent")
	parentTrace := parent.SpanContext().TraceID()
	_, err := platform.RunAgent(ctx, a, map[string]any{core.DefaultBindingName: word{Text: "lynx"}}, core.ProcessOptions{})
	parent.End()
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}

	for _, span := range exp.GetSpans() {
		if span.Name != "agent.listener.panic" {
			continue
		}
		if span.SpanContext.TraceID() != parentTrace {
			t.Fatalf("panic span trace = %s, want run trace %s", span.SpanContext.TraceID(), parentTrace)
		}
		return
	}
	t.Fatal("missing agent.listener.panic span")
}

func TestProcessContextPublishKeepsActionTrace(t *testing.T) {
	exp := installRuntimeTraceCapture(t)

	a := agent.New("action-event-trace").
		Actions(agent.NewAction("publish",
			func(_ context.Context, pc *core.ProcessContext, in word) (wordCount, error) {
				pc.Publish(event.ReplanRequested{
					BaseEvent: event.NewBaseEvent("manual"),
					Reason:    "custom",
				})
				return wordCount{Count: len(in.Text)}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[wordCount](core.Goal{Description: "counted"})).
		Build()

	platform := agent.NewPlatform(runtime.PlatformConfig{
		Extensions: []core.Extension{&customPanicListener{}},
	})
	mustDeploy(t, platform, a)

	ctx, parent := otel.Tracer("test/runtime").Start(context.Background(), "test-parent")
	parentTrace := parent.SpanContext().TraceID()
	_, err := platform.RunAgent(ctx, a, map[string]any{core.DefaultBindingName: word{Text: "lynx"}}, core.ProcessOptions{})
	parent.End()
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}

	for _, span := range exp.GetSpans() {
		if span.Name != "agent.listener.panic" {
			continue
		}
		if span.SpanContext.TraceID() != parentTrace {
			t.Fatalf("panic span trace = %s, want run trace %s", span.SpanContext.TraceID(), parentTrace)
		}
		return
	}
	t.Fatal("missing agent.listener.panic span")
}
