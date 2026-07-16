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
	"github.com/Tangerg/lynx/agent/hitl"
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
	if _, ok := e.(event.PlanningStarted); ok && l.done.CompareAndSwap(false, true) {
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

type modelCallPanicListener struct {
	done atomic.Bool
}

func (*modelCallPanicListener) Name() string { return "model-call-panic-listener" }

func (l *modelCallPanicListener) OnEvent(_ context.Context, e event.Event) {
	ev, ok := e.(event.ModelCallRecorded)
	if ok && ev.Call.Model == "ctx-model" && l.done.CompareAndSwap(false, true) {
		panic("model call listener failed")
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

func TestRuntimeEventPanicSpanKeepsRunTrace(t *testing.T) {
	exp := installRuntimeTraceCapture(t)

	a := agent.New(agent.AgentConfig{Name: "event-trace", Actions: []agent.Action{agent.NewAction("count", func(_ context.Context, _ *core.ProcessContext, in word) (wordCount, error) {
		return wordCount{Count: len(in.Text)}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "counted"})}})

	engine := agent.MustNewEngine(runtime.Config{
		Extensions: []core.Extension{&readyPanicListener{}},
	})
	mustDeploy(t, engine, a)

	ctx, parent := otel.Tracer("test/runtime").Start(context.Background(), "test-parent")
	parentTrace := parent.SpanContext().TraceID()
	_, err := engine.Run(ctx, a, map[string]any{core.DefaultBindingName: word{Text: "lynx"}}, core.ProcessOptions{})
	parent.End()
	if err != nil {
		t.Fatalf("Run: %v", err)
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

func TestProcessContextSuspendKeepsActionTrace(t *testing.T) {
	exp := installRuntimeTraceCapture(t)

	a := agent.New(agent.AgentConfig{Name: "await-event-trace", Actions: []agent.Action{agent.NewAction("wait", func(ctx context.Context, pc *core.ProcessContext, in word) (wordCount, error) {
		_, err := hitl.Interrupt[bool](ctx, "wait", "continue?")
		return wordCount{Count: len(in.Text)}, err
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "counted"})}})

	engine := agent.MustNewEngine(runtime.Config{
		Extensions: []core.Extension{&waitingPanicListener{}},
	})
	mustDeploy(t, engine, a)

	ctx, parent := otel.Tracer("test/runtime").Start(context.Background(), "test-parent")
	parentTrace := parent.SpanContext().TraceID()
	proc, err := engine.Run(ctx, a, map[string]any{core.DefaultBindingName: word{Text: "lynx"}}, core.ProcessOptions{})
	parent.End()
	if err != nil {
		t.Fatalf("Run: %v", err)
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

func TestProcessContextRecordModelCallKeepsActionTrace(t *testing.T) {
	exp := installRuntimeTraceCapture(t)

	a := agent.New(agent.AgentConfig{Name: "model-call-event-trace", Actions: []agent.Action{agent.NewAction("record", func(ctx context.Context, pc *core.ProcessContext, in word) (wordCount, error) {
		pc.RecordModelCall(ctx, core.ModelCall{Model: "ctx-model", Provider: "test", PromptTokens: int64(len(in.Text))})
		return wordCount{Count: len(in.Text)}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "counted"})}})

	engine := agent.MustNewEngine(runtime.Config{
		Extensions: []core.Extension{&modelCallPanicListener{}},
	})
	mustDeploy(t, engine, a)

	ctx, parent := otel.Tracer("test/runtime").Start(context.Background(), "test-parent")
	parentTrace := parent.SpanContext().TraceID()
	_, err := engine.Run(ctx, a, map[string]any{core.DefaultBindingName: word{Text: "lynx"}}, core.ProcessOptions{})
	parent.End()
	if err != nil {
		t.Fatalf("Run: %v", err)
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

	a := agent.New(agent.AgentConfig{Name: "action-event-trace", Actions: []agent.Action{agent.NewAction("publish", func(ctx context.Context, pc *core.ProcessContext, in word) (wordCount, error) {
		pc.Emit(ctx, event.ReplanRequested{Header: event.NewHeader("manual"), Reason: "custom"})
		return wordCount{Count: len(in.Text)}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "counted"})}})

	engine := agent.MustNewEngine(runtime.Config{
		Extensions: []core.Extension{&customPanicListener{}},
	})
	mustDeploy(t, engine, a)

	ctx, parent := otel.Tracer("test/runtime").Start(context.Background(), "test-parent")
	parentTrace := parent.SpanContext().TraceID()
	_, err := engine.Run(ctx, a, map[string]any{core.DefaultBindingName: word{Text: "lynx"}}, core.ProcessOptions{})
	parent.End()
	if err != nil {
		t.Fatalf("Run: %v", err)
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
