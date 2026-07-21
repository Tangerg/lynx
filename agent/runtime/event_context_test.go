package runtime_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

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

type lifecyclePanicListener struct {
	name string
	kind event.Kind
	done atomic.Bool
}

func (l *lifecyclePanicListener) Name() string { return l.name }

func (l *lifecyclePanicListener) OnEvent(_ context.Context, e event.Event) {
	if e.Kind() == l.kind && l.done.CompareAndSwap(false, true) {
		panic("lifecycle listener failed")
	}
}

func requireListenerPanicTrace(t *testing.T, exp *tracetest.InMemoryExporter, traceID trace.TraceID) {
	t.Helper()
	for _, span := range exp.GetSpans() {
		if span.Name != "agent.listener.panic" {
			continue
		}
		if span.SpanContext.TraceID() != traceID {
			t.Fatalf("panic span trace = %s, want caller trace %s", span.SpanContext.TraceID(), traceID)
		}
		return
	}
	t.Fatal("missing agent.listener.panic span")
}

func TestProcessCreatedEventKeepsRunTrace(t *testing.T) {
	exp := installRuntimeTraceCapture(t)
	a := buildSnapshotAgent()
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, a)
	listener := &lifecyclePanicListener{name: "created-panic-listener", kind: event.KindProcessCreated}

	ctx, parent := otel.Tracer("test/runtime").Start(t.Context(), "test-parent")
	parentTrace := parent.SpanContext().TraceID()
	_, err := engine.Run(ctx, a, core.Input(ssWord{Text: "lynx"}), core.ProcessOptions{
		Extensions: []core.Extension{listener},
	})
	parent.End()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	requireListenerPanicTrace(t, exp, parentTrace)
}

func TestProcessKilledEventKeepsCallerTrace(t *testing.T) {
	exp := installRuntimeTraceCapture(t)
	a := agent.New(agent.AgentConfig{Name: "kill-event-trace", Actions: []agent.Action{agent.NewAction("wait", func(ctx context.Context, _ *core.ProcessContext, _ word) (wordCount, error) {
		_, err := hitl.Interrupt[bool](ctx, "wait", "continue?")
		return wordCount{}, err
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "waited"})}})
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, a)
	listener := &lifecyclePanicListener{name: "killed-panic-listener", kind: event.KindProcessKilled}
	process, err := engine.Run(t.Context(), a, core.Input(word{Text: "lynx"}), core.ProcessOptions{
		Extensions: []core.Extension{listener},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if process.Status() != core.StatusWaiting {
		t.Fatalf("process status = %s, want %s", process.Status(), core.StatusWaiting)
	}
	exp.Reset()

	ctx, parent := otel.Tracer("test/runtime").Start(t.Context(), "test-parent")
	parentTrace := parent.SpanContext().TraceID()
	if err := engine.Kill(ctx, process.ID()); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	parent.End()

	requireListenerPanicTrace(t, exp, parentTrace)
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
	_, err := engine.Run(ctx, a, core.Input(word{Text: "lynx"}), core.ProcessOptions{})
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
	proc, err := engine.Run(ctx, a, core.Input(word{Text: "lynx"}), core.ProcessOptions{})
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
		if err := pc.RecordModelCall(ctx, core.ModelCall{Model: "ctx-model", Provider: "test", PromptTokens: int64(len(in.Text))}); err != nil {
			return wordCount{}, err
		}
		return wordCount{Count: len(in.Text)}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "counted"})}})

	engine := agent.MustNewEngine(runtime.Config{
		Extensions: []core.Extension{&modelCallPanicListener{}},
	})
	mustDeploy(t, engine, a)

	ctx, parent := otel.Tracer("test/runtime").Start(context.Background(), "test-parent")
	parentTrace := parent.SpanContext().TraceID()
	_, err := engine.Run(ctx, a, core.Input(word{Text: "lynx"}), core.ProcessOptions{})
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
