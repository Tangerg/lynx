package terminal

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/programtest"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
)

type runtimeChangeSourceStub struct {
	events       chan changefeed.Event
	subscription chan changefeed.Subscription
	applied      chan changefeed.Event
}

func (stub *runtimeChangeSourceStub) Supports(topic changefeed.Topic) bool {
	return slices.Contains([]changefeed.Topic{
		changefeed.SessionsChanged, changefeed.RunsChanged,
		changefeed.StateChanged, changefeed.InterruptsChanged,
	}, topic)
}

func (stub *runtimeChangeSourceStub) Subscribe(ctx context.Context, subscription changefeed.Subscription) (changefeed.EventStream, error) {
	select {
	case stub.subscription <- subscription:
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
	return func(yield func(changefeed.Event, error) bool) {
		for {
			select {
			case event := <-stub.events:
				if !yield(event, nil) {
					return
				}
				if stub.applied != nil {
					select {
					case stub.applied <- event:
					case <-ctx.Done():
						return
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}, nil
}

func TestRuntimeInvalidationDefersColdReplacementUntilTheStreamSettles(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockStarted{Block: agent.Block{ID: "thinking", Kind: agent.BlockReasoning}}},
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	backend := &snapshotCountingRuntime{Runtime: base, readSignal: make(chan struct{}, 16)}
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeChanges(t, backend, source, "ses_demo_1")

	host.Shows(t, "Ask lyra")
	select {
	case <-source.subscription:
	case <-time.After(3 * time.Second):
		t.Fatal("runtime invalidation subscription was not opened")
	}
	host.Type("keep the stream alive")
	host.Press(input.Enter)
	host.Shows(t, "thinking")
	snapshot, err := base.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	title := "Changed while streaming"
	if _, err := base.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: snapshot.Session.ID, Title: &title, ExpectedRevision: snapshot.Session.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	baseline := backend.reads.Load()
	drainSignals(backend.readSignal)
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	select {
	case <-source.applied:
	case <-time.After(3 * time.Second):
		t.Fatal("session invalidation was not applied")
	}
	if got := backend.reads.Load(); got != baseline {
		t.Fatalf("active stream triggered a cold session read: reads %d, want %d", got, baseline)
	}
	host.Press(input.Esc)
	host.Shows(t, "canceled")
	awaitSignal(t, backend.readSignal, "the deferred authoritative session read")
	host.Shows(t, title)
	if backend.reads.Load() <= baseline {
		t.Fatal("settled stream did not reconcile the deferred invalidation")
	}
	stop()
}

type snapshotCountingRuntime struct {
	*mock.Runtime
	reads      atomic.Int32
	readSignal chan struct{}
}

func (runtime *snapshotCountingRuntime) GetSession(ctx context.Context, id string) (agent.SessionSnapshot, error) {
	runtime.reads.Add(1)
	if runtime.readSignal != nil {
		select {
		case runtime.readSignal <- struct{}{}:
		default:
		}
	}
	return runtime.Runtime.GetSession(ctx, id)
}

func runUIWithRuntimeChanges(t *testing.T, runtime agent.Runtime, source changefeed.Source, sessionID string) (*programtest.Host, func()) {
	t.Helper()
	host := programtest.New(t, 96, 28)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{Runtime: runtime, Changes: source, SessionID: sessionID, Host: host})
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("terminal session stopped with %v", err)
			}
		})
	}
	t.Cleanup(stop)
	return host, stop
}

func TestRuntimeInvalidationsRefetchTheCurrentAuthoritativeSession(t *testing.T) {
	backend := &snapshotCountingRuntime{Runtime: mock.New(), readSignal: make(chan struct{}, 16)}
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 8), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 8),
	}
	host, stop := runUIWithRuntimeChanges(t, backend, source, "ses_demo_1")

	host.Shows(t, "Ask lyra")
	var subscription changefeed.Subscription
	select {
	case subscription = <-source.subscription:
	case <-t.Context().Done():
		t.Fatal("runtime invalidation subscription was not opened")
	}
	wantTopics := []changefeed.Topic{
		changefeed.SessionsChanged, changefeed.RunsChanged,
		changefeed.StateChanged, changefeed.InterruptsChanged,
	}
	if !slices.Equal(subscription.Topics, wantTopics) || len(subscription.Watches) != 0 {
		t.Fatalf("subscription = %+v", subscription)
	}
	host.Until(t, "the initial attach-first cold session read", func() bool {
		return backend.reads.Load() >= 2 && host.Repaint()
	})

	snapshot, err := backend.Runtime.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	title := "Renamed elsewhere"
	if _, err := backend.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: snapshot.Session.ID, Title: &title, ExpectedRevision: snapshot.Session.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	baseline := backend.reads.Load()
	drainSignals(backend.readSignal)
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitSignal(t, source.applied, "sessions.changed delivery")
	awaitSignal(t, backend.readSignal, "sessions.changed authoritative read")
	host.Shows(t, "Renamed elsewhere")
	if backend.reads.Load() <= baseline {
		t.Fatal("sessions.changed did not trigger an authoritative session read")
	}

	for index, topic := range []changefeed.Topic{
		changefeed.RunsChanged, changefeed.StateChanged, changefeed.InterruptsChanged,
	} {
		baseline = backend.reads.Load()
		drainSignals(backend.readSignal)
		source.events <- changefeed.Event{
			Type: changefeed.EventType(topic), Sequence: uint64(index + 2),
			SessionIDs: []string{"ses_demo_1"},
		}
		awaitSignal(t, source.applied, string(topic)+" delivery")
		awaitSignal(t, backend.readSignal, string(topic)+" authoritative read")
		if backend.reads.Load() <= baseline {
			t.Fatalf("%s did not trigger an authoritative session read", topic)
		}
	}
	stop()
}

func TestDeletedActiveSessionIsReplacedFromItsWorkspace(t *testing.T) {
	backend := &snapshotCountingRuntime{Runtime: mock.New(), readSignal: make(chan struct{}, 8)}
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeChanges(t, backend, source, "ses_demo_1")
	host.Shows(t, "Ask lyra")
	awaitSignal(t, source.subscription, "runtime invalidation subscription")
	host.Until(t, "the attach-first session read", func() bool {
		return backend.reads.Load() >= 2 && host.Repaint()
	})
	if err := backend.DeleteSession(t.Context(), agent.DeleteSession{SessionID: "ses_demo_1"}); err != nil {
		t.Fatal(err)
	}
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitSignal(t, source.applied, "deleted-session invalidation")
	host.Shows(t, "Untitled session")
	stop()
}

func TestMatchingInterruptInvalidationPreservesTheOpenApproval(t *testing.T) {
	base := mock.New()
	base.Instant = true
	answers := make(chan []agent.InterruptAnswer, 1)
	base.Script = func(string) mock.Script {
		return mock.Script{
			Interactions: []agent.Interaction{agent.Approval{
				ItemID: "approval_invalidation", Title: "Run generated command",
				Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Command: "go test ./...", Status: agent.ToolRunning},
			}},
			Continue: func(provided []agent.InterruptAnswer) []mock.Step {
				answers <- provided
				return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	backend := &snapshotCountingRuntime{Runtime: base, readSignal: make(chan struct{}, 8)}
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeChanges(t, backend, source, "ses_demo_1")
	host.Shows(t, "Ask lyra")
	awaitSignal(t, source.subscription, "runtime invalidation subscription")
	host.Type("test after the side-channel update")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")
	drainSignals(backend.readSignal)
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.InterruptsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitSignal(t, source.applied, "interrupts.changed delivery")
	awaitSignal(t, backend.readSignal, "interrupts.changed authoritative read")
	host.Shows(t, "Tool approval")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	if provided := <-answers; len(provided) != 1 {
		t.Fatalf("approval responses = %+v", provided)
	}
	stop()
}

func drainSignals[T any](signals <-chan T) {
	for {
		select {
		case <-signals:
		default:
			return
		}
	}
}

func awaitSignal[T any](t *testing.T, signals <-chan T, what string) {
	t.Helper()
	select {
	case <-signals:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for " + what)
	}
}

func TestRuntimeChangeMonitorTurnsASequenceGapIntoFullResync(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 2), subscription: make(chan changefeed.Subscription, 1),
	}
	source.events <- changefeed.Event{Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1}
	source.events <- changefeed.Event{Type: changefeed.EventType(changefeed.RunsChanged), Sequence: 3}
	var events []changefeed.Event
	var resyncs [][]changefeed.Topic
	monitor := runtimeChangeMonitor{
		source: source,
		applyEvent: func(event changefeed.Event) error {
			events = append(events, event)
			if event.Sequence == 3 {
				cancel()
			}
			return nil
		},
		applyResync: func(topics []changefeed.Topic) error {
			resyncs = append(resyncs, slices.Clone(topics))
			return nil
		},
	}
	monitor.run(ctx)
	if len(events) != 2 || events[0].Sequence != 1 || events[1].Sequence != 3 {
		t.Fatalf("events = %+v", events)
	}
	if len(resyncs) != 2 || !slices.Equal(resyncs[0], resyncs[1]) ||
		!slices.Equal(resyncs[0], []changefeed.Topic{
			changefeed.SessionsChanged, changefeed.RunsChanged,
			changefeed.StateChanged, changefeed.InterruptsChanged,
		}) {
		t.Fatalf("resyncs = %+v", resyncs)
	}
}

func TestRuntimeInvalidationScope(t *testing.T) {
	tests := []struct {
		name  string
		event changefeed.Event
		want  bool
	}{
		{name: "foreign session", event: changefeed.Event{Type: changefeed.EventType(changefeed.SessionsChanged), SessionIDs: []string{"other"}}},
		{name: "current session", event: changefeed.Event{Type: changefeed.EventType(changefeed.SessionsChanged), SessionIDs: []string{"session"}}, want: true},
		{name: "unscoped state", event: changefeed.Event{Type: changefeed.EventType(changefeed.StateChanged)}, want: true},
		{name: "foreign run", event: changefeed.Event{Type: changefeed.EventType(changefeed.RunsChanged), RunIDs: []string{"other"}}},
		{name: "current run", event: changefeed.Event{Type: changefeed.EventType(changefeed.InterruptsChanged), RunIDs: []string{"run"}}, want: true},
		{name: "files", event: changefeed.Event{Type: changefeed.EventType(changefeed.FilesChanged)}},
		{name: "session resync", event: changefeed.Event{Type: changefeed.Resync, Topics: []changefeed.Topic{changefeed.SessionsChanged}}, want: true},
		{name: "schedule resync", event: changefeed.Event{Type: changefeed.Resync, Topics: []changefeed.Topic{changefeed.SchedulesChanged}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := invalidationAffectsSession(test.event, "session", "run"); got != test.want {
				t.Fatalf("invalidationAffectsSession() = %t, want %t", got, test.want)
			}
		})
	}
}
