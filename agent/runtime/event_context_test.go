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

func (l *readyPanicListener) OnEvent(_ context.Context, e event.Event) {
	if _, ok := e.(event.ReadyToPlan); ok && l.done.CompareAndSwap(false, true) {
		panic("ready listener failed")
	}
}

type customPanicListener struct {
	done atomic.Bool
}

func (*customPanicListener) Name() string { return "custom-panic-listener" }

func (l *customPanicListener) OnEvent(_ context.Context, e event.Event) {
	ev, ok := e.(event.ReplanRequested)
	if ok && ev.Reason == "custom" && l.done.CompareAndSwap(false, true) {
		panic("custom listener failed")
	}
}

type invocationPanicListener struct {
	done atomic.Bool
}

func (*invocationPanicListener) Name() string { return "invocation-panic-listener" }

func (l *invocationPanicListener) OnEvent(_ context.Context, e event.Event) {
	ev, ok := e.(event.LLMInvocationRecorded)
	if ok && ev.Invocation.Model == "ctx-model" && l.done.CompareAndSwap(false, true) {
		panic("invocation listener failed")
	}
}

type waitingPanicListener struct {
	done atomic.Bool
}

func (*waitingPanicListener) Name() string { return "waiting-panic-listener" }

func (l *waitingPanicListener) OnEvent(_ context.Context, e event.Event) {
	if _, ok := e.(event.ProcessWaiting); ok && l.done.CompareAndSwap(false, true) {
		panic("waiting listener failed")
	}
}

type traceAwaitable struct{ id string }

func (a traceAwaitable) ID() string     { return a.id }
func (a traceAwaitable) PromptAny() any { return "continue?" }
func (a traceAwaitable) OnResponseAny(any) (core.ResponseImpact, error) {
	return core.ImpactUnchanged, nil
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

func TestProcessContextAwaitInputKeepsActionTrace(t *testing.T) {
	exp := installRuntimeTraceCapture(t)

	a := agent.New("await-event-trace").
		Actions(agent.NewAction("wait",
			func(ctx context.Context, pc *core.ProcessContext, in word) (wordCount, error) {
				pc.AwaitInput(ctx, traceAwaitable{id: "wait"})
				return wordCount{Count: len(in.Text)}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[wordCount](core.Goal{Description: "counted"})).
		Build()

	platform := agent.NewPlatform(runtime.PlatformConfig{
		Extensions: []core.Extension{&waitingPanicListener{}},
	})
	mustDeploy(t, platform, a)

	ctx, parent := otel.Tracer("test/runtime").Start(context.Background(), "test-parent")
	parentTrace := parent.SpanContext().TraceID()
	proc, err := platform.RunAgent(ctx, a, map[string]any{core.DefaultBindingName: word{Text: "lynx"}}, core.ProcessOptions{})
	parent.End()
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if proc.Status() != core.StatusWaiting {
		t.Fatalf("process status = %s, want %s", proc.Status(), core.StatusWaiting)
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

func TestProcessContextRecordInvocationKeepsActionTrace(t *testing.T) {
	exp := installRuntimeTraceCapture(t)

	a := agent.New("invocation-event-trace").
		Actions(agent.NewAction("record",
			func(ctx context.Context, pc *core.ProcessContext, in word) (wordCount, error) {
				pc.RecordLLMInvocation(ctx, core.LLMInvocation{
					Model:        "ctx-model",
					Provider:     "test",
					PromptTokens: int64(len(in.Text)),
				})
				return wordCount{Count: len(in.Text)}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[wordCount](core.Goal{Description: "counted"})).
		Build()

	platform := agent.NewPlatform(runtime.PlatformConfig{
		Extensions: []core.Extension{&invocationPanicListener{}},
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
			func(ctx context.Context, pc *core.ProcessContext, in word) (wordCount, error) {
				pc.Publish(ctx, event.ReplanRequested{
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
