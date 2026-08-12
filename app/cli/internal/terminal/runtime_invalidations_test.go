package terminal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/programtest"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/reconnect"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
	"github.com/Tangerg/lynx/app/cli/internal/workspace"
)

type runtimeChangeSourceStub struct {
	events       chan changefeed.Event
	subscribeErr chan error
	streamErrors chan error
	streamClosed <-chan struct{}
	subscription chan changefeed.Subscription
	applied      chan changefeed.Event
	supported    []changefeed.Topic
}

type runtimeSubscriptionRegistration struct {
	subscription changefeed.Subscription
	events       chan changefeed.Event
}

type partitionedRuntimeChangeSourceStub struct {
	supported     []changefeed.Topic
	registrations chan runtimeSubscriptionRegistration
}

func (stub *partitionedRuntimeChangeSourceStub) Supports(topic changefeed.Topic) bool {
	return slices.Contains(stub.supported, topic)
}

func (stub *partitionedRuntimeChangeSourceStub) Subscribe(
	ctx context.Context,
	subscription changefeed.Subscription,
) (changefeed.EventStream, error) {
	registration := runtimeSubscriptionRegistration{
		subscription: subscription,
		events:       make(chan changefeed.Event, 4),
	}
	select {
	case stub.registrations <- registration:
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
	return func(yield func(changefeed.Event, error) bool) {
		for {
			select {
			case event := <-registration.events:
				if !yield(event, nil) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}, nil
}

func (stub *runtimeChangeSourceStub) Supports(topic changefeed.Topic) bool {
	if stub.supported != nil {
		return slices.Contains(stub.supported, topic)
	}
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
	select {
	case err := <-stub.subscribeErr:
		return nil, err
	default:
	}
	return func(yield func(changefeed.Event, error) bool) {
		for {
			select {
			case <-stub.streamClosed:
				return
			case err := <-stub.streamErrors:
				yield(changefeed.Event{}, err)
				return
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

func TestRuntimeChangeMonitorStopsOnAnIncompatibleSubscription(t *testing.T) {
	t.Parallel()
	source := &runtimeChangeSourceStub{
		events:       make(chan changefeed.Event),
		subscribeErr: make(chan error, 1),
		subscription: make(chan changefeed.Subscription, 1),
	}
	source.subscribeErr <- agent.ErrIncompatibleRuntime

	err := (runtimeChangeMonitor{source: source}).run(t.Context())
	if !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("run error = %v, want ErrIncompatibleRuntime", err)
	}
	if subscriptions := len(source.subscription); subscriptions != 1 {
		t.Fatalf("subscriptions = %d, want exactly one", subscriptions)
	}
}

func TestRuntimeChangeMonitorStopsOnAPermanentSubscriptionFailure(t *testing.T) {
	t.Parallel()
	permanent := errors.New("invalid change subscription")
	source := &runtimeChangeSourceStub{
		events:       make(chan changefeed.Event),
		subscribeErr: make(chan error, 1),
		subscription: make(chan changefeed.Subscription, 2),
	}
	source.subscribeErr <- permanent

	err := (runtimeChangeMonitor{source: source}).run(t.Context())
	if !errors.Is(err, permanent) {
		t.Fatalf("run error = %v, want permanent subscription failure", err)
	}
	if subscriptions := len(source.subscription); subscriptions != 1 {
		t.Fatalf("subscriptions = %d, want exactly one", subscriptions)
	}
}

func TestRuntimeChangeMonitorStopsOnAnIncompatibleStream(t *testing.T) {
	t.Parallel()
	source := &runtimeChangeSourceStub{
		events:       make(chan changefeed.Event),
		streamErrors: make(chan error, 1),
		subscription: make(chan changefeed.Subscription, 1),
	}
	source.streamErrors <- agent.ErrIncompatibleRuntime

	err := (runtimeChangeMonitor{source: source}).run(t.Context())
	if !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("run error = %v, want ErrIncompatibleRuntime", err)
	}
	if subscriptions := len(source.subscription); subscriptions != 1 {
		t.Fatalf("subscriptions = %d, want exactly one", subscriptions)
	}
}

func TestRuntimeChangeMonitorReconnectsWhenAStreamClosesUnexpectedly(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	closed := make(chan struct{})
	close(closed)
	source := &runtimeChangeSourceStub{
		events:       make(chan changefeed.Event),
		streamClosed: closed,
		subscription: make(chan changefeed.Subscription, 2),
	}
	done := make(chan error, 1)
	go func() { done <- (runtimeChangeMonitor{source: source}).run(ctx) }()

	awaitSignal(t, source.subscription, "initial runtime subscription")
	awaitSignal(t, source.subscription, "replacement runtime subscription")
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context cancellation", err)
	}
}

func TestRuntimeChangeMonitorBacksOffRepeatedEmptyStreams(t *testing.T) {
	closed := make(chan struct{})
	close(closed)
	source := &runtimeChangeSourceStub{
		events:       make(chan changefeed.Event),
		streamClosed: closed,
		subscription: make(chan changefeed.Subscription, 8),
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	monitor := runtimeChangeMonitor{
		source:   source,
		recovery: reconnect.Backoff{Base: 20 * time.Millisecond, Maximum: 40 * time.Millisecond},
	}
	done := make(chan error, 1)
	started := time.Now()
	go func() { done <- monitor.run(ctx) }()

	awaitSignal(t, source.subscription, "initial runtime subscription")
	awaitSignal(t, source.subscription, "first replacement runtime subscription")
	awaitSignal(t, source.subscription, "second replacement runtime subscription")
	if elapsed := time.Since(started); elapsed < 55*time.Millisecond {
		t.Fatalf("three empty streams retried after %s, before the cumulative backoff", elapsed)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context cancellation", err)
	}
}

func TestRuntimeChangeMonitorPartitionsTopicsAtTheNegotiatedLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	supported := []changefeed.Topic{
		changefeed.SessionsChanged,
		changefeed.RunsChanged,
		changefeed.StateChanged,
		changefeed.InterruptsChanged,
		changefeed.SkillsChanged,
	}
	source := &partitionedRuntimeChangeSourceStub{
		supported:     supported,
		registrations: make(chan runtimeSubscriptionRegistration, 3),
	}
	resyncs := make(chan []changefeed.Topic, 4)
	applied := make(chan changefeed.Event, 3)
	monitor := runtimeChangeMonitor{
		source: source, includeSkills: true,
		subscriptionLimits: changefeed.SubscriptionLimits{MaxTopics: 2, MaxWatches: 1},
		applyResync: func(topics []changefeed.Topic) error {
			resyncs <- slices.Clone(topics)
			return nil
		},
		applyEvent: func(event changefeed.Event) error {
			applied <- event
			return nil
		},
	}
	done := make(chan error, 1)
	go func() { done <- monitor.run(ctx) }()

	registrations := make([]runtimeSubscriptionRegistration, 0, 3)
	for range 3 {
		registrations = append(registrations, awaitValue(t, source.registrations, "partitioned subscription"))
	}
	for range 3 {
		awaitValue(t, resyncs, "partition initial resync")
	}
	slices.SortFunc(registrations, func(left, right runtimeSubscriptionRegistration) int {
		return strings.Compare(string(left.subscription.Topics[0]), string(right.subscription.Topics[0]))
	})
	var subscribed []changefeed.Topic
	for _, registration := range registrations {
		if len(registration.subscription.Topics) > 2 {
			t.Fatalf("subscription topics = %v, exceeds negotiated limit", registration.subscription.Topics)
		}
		subscribed = append(subscribed, registration.subscription.Topics...)
		registration.events <- changefeed.Event{
			Type: changefeed.EventType(registration.subscription.Topics[0]), Sequence: 1,
		}
	}
	slices.Sort(subscribed)
	wantTopics := slices.Clone(supported)
	slices.Sort(wantTopics)
	if !slices.Equal(subscribed, wantTopics) {
		t.Fatalf("subscribed topics = %v, want %v", subscribed, wantTopics)
	}
	for range 3 {
		awaitValue(t, applied, "partitioned event")
	}
	select {
	case unexpected := <-resyncs:
		t.Fatalf("independent sequence-one frames triggered a gap resync for %v", unexpected)
	default:
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context cancellation", err)
	}
}

func TestRuntimeChangeMonitorResyncsOnlyThePartitionWithASequenceGap(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	source := &partitionedRuntimeChangeSourceStub{
		supported: []changefeed.Topic{
			changefeed.SessionsChanged, changefeed.RunsChanged,
			changefeed.StateChanged, changefeed.InterruptsChanged,
		},
		registrations: make(chan runtimeSubscriptionRegistration, 2),
	}
	resyncs := make(chan []changefeed.Topic, 3)
	monitor := runtimeChangeMonitor{
		source:             source,
		subscriptionLimits: changefeed.SubscriptionLimits{MaxTopics: 2, MaxWatches: 1},
		applyResync: func(topics []changefeed.Topic) error {
			resyncs <- slices.Clone(topics)
			return nil
		},
	}
	done := make(chan error, 1)
	go func() { done <- monitor.run(ctx) }()

	first := awaitValue(t, source.registrations, "first partition")
	awaitValue(t, source.registrations, "second partition")
	for range 2 {
		awaitValue(t, resyncs, "partition initial resync")
	}
	first.events <- changefeed.Event{
		Type: changefeed.EventType(first.subscription.Topics[0]), Sequence: 2,
	}
	gapScope := awaitValue(t, resyncs, "partition gap resync")
	if !slices.Equal(gapScope, first.subscription.Topics) {
		t.Fatalf("gap resync scope = %v, want %v", gapScope, first.subscription.Topics)
	}
	select {
	case unexpected := <-resyncs:
		t.Fatalf("gap in one partition resynced another scope: %v", unexpected)
	case <-time.After(25 * time.Millisecond):
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context cancellation", err)
	}
}

func TestRuntimeChangeMonitorAssignsTheWorkspaceWatchToOnePartition(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	source := &partitionedRuntimeChangeSourceStub{
		supported: []changefeed.Topic{
			changefeed.FilesChanged, changefeed.SessionsChanged,
			changefeed.RunsChanged, changefeed.StateChanged, changefeed.InterruptsChanged,
		},
		registrations: make(chan runtimeSubscriptionRegistration, 5),
	}
	var reads atomic.Int32
	filesApplied := make(chan struct{}, 1)
	monitor := runtimeChangeMonitor{
		workspace: "/workspace", source: source, watchFiles: true,
		repository: changeReaderFunc(func(context.Context, string) ([]workspace.Change, error) {
			reads.Add(1)
			return nil, nil
		}),
		subscriptionLimits: changefeed.SubscriptionLimits{MaxTopics: 1, MaxWatches: 1},
		applyFiles: func([]workspace.Change) error {
			filesApplied <- struct{}{}
			return nil
		},
	}
	done := make(chan error, 1)
	go func() { done <- monitor.run(ctx) }()

	watchSubscriptions := 0
	for range 5 {
		registration := awaitValue(t, source.registrations, "single-topic subscription")
		if len(registration.subscription.Watches) == 0 {
			continue
		}
		watchSubscriptions++
		if !slices.Equal(registration.subscription.Topics, []changefeed.Topic{changefeed.FilesChanged}) ||
			!slices.Equal(registration.subscription.Watches, []changefeed.Watch{{ID: workspaceWatchID, Workspace: "/workspace"}}) {
			t.Fatalf("file subscription = %+v", registration.subscription)
		}
	}
	awaitSignal(t, filesApplied, "initial workspace projection")
	if watchSubscriptions != 1 || reads.Load() != 1 {
		t.Fatalf("watch subscriptions = %d, workspace reads = %d; want one of each", watchSubscriptions, reads.Load())
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context cancellation", err)
	}
}

func TestRuntimeChangeMonitorKeepsGlobalEventsAliveWithoutVersionControl(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	source := &runtimeChangeSourceStub{
		events:       make(chan changefeed.Event, 2),
		subscription: make(chan changefeed.Subscription, 1),
		supported:    []changefeed.Topic{changefeed.FilesChanged, changefeed.SessionsChanged},
	}
	var reads atomic.Int32
	filesApplied := make(chan []workspace.Change, 2)
	eventsApplied := make(chan changefeed.Event, 1)
	monitor := runtimeChangeMonitor{
		workspace: "/workspace", source: source, watchFiles: true,
		repository: changeReaderFunc(func(context.Context, string) ([]workspace.Change, error) {
			if reads.Add(1) == 1 {
				return nil, workspace.ErrVersionControlUnavailable
			}
			return []workspace.Change{{Path: "main.go", Status: workspace.FileStatusModified}}, nil
		}),
		applyFiles: func(changes []workspace.Change) error {
			filesApplied <- slices.Clone(changes)
			return nil
		},
		applyEvent: func(event changefeed.Event) error {
			eventsApplied <- event
			return nil
		},
	}
	done := make(chan error, 1)
	go func() { done <- monitor.run(ctx) }()

	awaitValue(t, source.subscription, "runtime subscription")
	if initial := awaitValue(t, filesApplied, "empty non-git projection"); len(initial) != 0 {
		t.Fatalf("initial file projection = %+v, want empty", initial)
	}
	sessionEvent := changefeed.Event{
		Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	source.events <- sessionEvent
	if applied := awaitValue(t, eventsApplied, "session invalidation"); applied.Type != sessionEvent.Type {
		t.Fatalf("applied event = %+v, want %+v", applied, sessionEvent)
	}
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.FilesChanged), Sequence: 2,
		Workspace: "/workspace",
	}
	if recovered := awaitValue(t, filesApplied, "recovered git projection"); len(recovered) != 1 || recovered[0].Path != "main.go" {
		t.Fatalf("recovered file projection = %+v", recovered)
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context cancellation", err)
	}
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

func TestSessionChangeSettlementReconcilesItsDeferredInvalidation(t *testing.T) {
	tests := []struct {
		name      string
		changeErr error
		cancel    bool
	}{
		{name: "canceled", cancel: true},
		{name: "failed", changeErr: errors.New("session creation failed")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := mock.New()
			counting := &snapshotCountingRuntime{Runtime: base, readSignal: make(chan struct{}, 16)}
			backend := &blockingSessionChangeRuntime{
				Runtime:       counting,
				blockCreateAt: 1,
				changeErr:     test.changeErr,
				changeStarted: make(chan struct{}),
				releaseChange: make(chan struct{}),
			}
			source := &runtimeChangeSourceStub{
				events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
				applied: make(chan changefeed.Event, 1),
			}
			host, stop := runUIWithRuntimeChanges(t, backend, source, "ses_demo_1")
			host.Shows(t, "Ask lyra")
			awaitSignal(t, source.subscription, "runtime invalidation subscription")
			host.Type("/new")
			host.Press(input.Enter)
			awaitSignal(t, backend.changeStarted, "session creation")

			snapshot, err := base.GetSession(t.Context(), "ses_demo_1")
			if err != nil {
				t.Fatal(err)
			}
			title := "Changed during " + test.name + " session creation"
			if _, err := base.UpdateSession(t.Context(), agent.UpdateSession{
				SessionID: snapshot.Session.ID, Title: &title, ExpectedRevision: snapshot.Session.Revision,
			}); err != nil {
				t.Fatal(err)
			}
			baseline := counting.reads.Load()
			drainSignals(counting.readSignal)
			source.events <- changefeed.Event{
				Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1,
				SessionIDs: []string{"ses_demo_1"},
			}
			awaitSignal(t, source.applied, "session invalidation")
			if got := counting.reads.Load(); got != baseline {
				t.Fatalf("active session change triggered a cold session read: reads %d, want %d", got, baseline)
			}

			if test.cancel {
				host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
			} else {
				close(backend.releaseChange)
			}
			awaitSignal(t, counting.readSignal, "deferred authoritative session read")
			host.Shows(t, title)
			stop()
		})
	}
}

type snapshotCountingRuntime struct {
	*mock.Runtime
	reads      atomic.Int32
	failures   atomic.Int32
	failure    error
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
	for remaining := runtime.failures.Load(); remaining > 0; remaining = runtime.failures.Load() {
		if runtime.failures.CompareAndSwap(remaining, remaining-1) {
			return agent.SessionSnapshot{}, runtime.failure
		}
	}
	return runtime.Runtime.GetSession(ctx, id)
}

func runUIWithRuntimeChanges(t *testing.T, runtime agent.Runtime, source changefeed.Source, sessionID string) (*programtest.Host, func()) {
	return runUIWithRuntimeChangeServices(t, runtime, nil, source, sessionID)
}

func runUIWithRuntimeChangeServices(t *testing.T, runtime agent.Runtime, workspaces workspace.Service, source changefeed.Source, sessionID string) (*programtest.Host, func()) {
	t.Helper()
	host := programtest.New(t, 96, 28)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{Runtime: runtime, Workspaces: workspaces, Changes: source, SessionID: sessionID, Host: host})
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

func TestRuntimeInvalidationRecoversAReadOnlyProjectionAfterTransientFailures(t *testing.T) {
	base := mock.New()
	backend := &snapshotCountingRuntime{
		Runtime:    base,
		failure:    fmt.Errorf("temporary session projection failure: %w", agent.ErrDisconnected),
		readSignal: make(chan struct{}, 16),
	}
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
	}
	host, stop := runUIWithRuntimeChanges(t, backend, source, "ses_demo_1")
	host.Shows(t, "Ask lyra")
	awaitSignal(t, source.subscription, "runtime invalidation subscription")
	host.Until(t, "the initial attach-first session read", func() bool {
		return backend.reads.Load() >= 2 && host.Repaint()
	})

	snapshot, err := base.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	title := "Recovered without another event"
	if _, err := base.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: snapshot.Session.ID, Title: &title, ExpectedRevision: snapshot.Session.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	baseline := backend.reads.Load()
	backend.failures.Store(2)
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	host.Shows(t, title)
	if got := backend.reads.Load() - baseline; got < 3 {
		t.Fatalf("session reads after invalidation = %d, want at least 3", got)
	}
	stop()
}

func TestInvalidatedSessionReadDoesNotRetryAPermanentFailure(t *testing.T) {
	permanent := errors.New("invalid session projection")
	backend := &snapshotCountingRuntime{Runtime: mock.New(), failure: permanent}
	backend.failures.Store(2)

	_, err := (&app{runtime: backend}).readInvalidatedSession(t.Context(), "ses_demo_1")
	if !errors.Is(err, permanent) {
		t.Fatalf("readInvalidatedSession error = %v, want permanent failure", err)
	}
	if reads := backend.reads.Load(); reads != 1 {
		t.Fatalf("session reads = %d, want exactly one", reads)
	}
}

func TestExternalWorkspaceChangeRebindsTheRuntimeWatch(t *testing.T) {
	backend := mock.New()
	service := &workspacePathRecordingService{workspaceServiceStub: newWorkspaceServiceStub(), paths: make(chan string, 4)}
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 4),
		supported: []changefeed.Topic{
			changefeed.FilesChanged, changefeed.SessionsChanged, changefeed.RunsChanged,
			changefeed.StateChanged, changefeed.InterruptsChanged,
		},
	}
	host, stop := runUIWithRuntimeChangeServices(t, backend, service, source, "ses_demo_1")
	host.Shows(t, "Ask lyra")
	initial := awaitSubscription(t, source.subscription, "initial runtime subscription")
	if len(initial.Watches) != 1 || initial.Watches[0].Workspace != "/tmp/demo/store" {
		t.Fatalf("initial workspace watch = %+v", initial.Watches)
	}
	if initialPath := awaitWorkspacePath(t, service.paths, "initial workspace refresh"); initialPath != "/tmp/demo/store" {
		t.Fatalf("initial workspace refresh = %s", initialPath)
	}

	snapshot, err := backend.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	workspacePath := t.TempDir()
	if _, err := backend.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: snapshot.Session.ID, Workspace: &workspacePath, ExpectedRevision: snapshot.Session.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	rebound := awaitSubscription(t, source.subscription, "rebound runtime subscription")
	if len(rebound.Watches) != 1 || rebound.Watches[0].Workspace != workspacePath {
		t.Fatalf("rebound workspace watch = %+v, want %s", rebound.Watches, workspacePath)
	}
	if reboundPath := awaitWorkspacePath(t, service.paths, "rebound workspace refresh"); reboundPath != workspacePath {
		t.Fatalf("rebound workspace refresh = %s, want %s", reboundPath, workspacePath)
	}
	stop()
}

type workspacePathRecordingService struct {
	*workspaceServiceStub
	paths chan string
}

func (service *workspacePathRecordingService) Changes(ctx context.Context, path string) ([]workspace.Change, error) {
	select {
	case service.paths <- path:
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
	return service.workspaceServiceStub.Changes(ctx, path)
}

func awaitSubscription(t *testing.T, subscriptions <-chan changefeed.Subscription, what string) changefeed.Subscription {
	t.Helper()
	select {
	case subscription := <-subscriptions:
		return subscription
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for " + what)
		return changefeed.Subscription{}
	}
}

func awaitWorkspacePath(t *testing.T, paths <-chan string, what string) string {
	t.Helper()
	select {
	case path := <-paths:
		return path
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for " + what)
		return ""
	}
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
		event := changefeed.Event{
			Type: changefeed.EventType(topic), Sequence: uint64(index + 2),
			SessionIDs: []string{"ses_demo_1"},
		}
		if topic == changefeed.StateChanged {
			event.StateKey = changefeed.StatePlan
		}
		source.events <- event
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

func TestDeletedActiveSessionTransfersItsUnsentDraftToTheReplacement(t *testing.T) {
	base := mock.New()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	stateDirectory := t.TempDir()
	host, stop := runUIFromConfig(t, Config{
		Runtime: base, Changes: source, SessionID: "ses_demo_1",
		StateDirectory: stateDirectory,
	})
	host.Shows(t, "Ask lyra")
	awaitSignal(t, source.subscription, "runtime invalidation subscription")
	host.Type("unsent draft survives forced replacement")
	host.Shows(t, "unsent draft survives forced replacement")

	if err := base.DeleteSession(t.Context(), agent.DeleteSession{SessionID: "ses_demo_1"}); err != nil {
		t.Fatal(err)
	}
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitSignal(t, source.applied, "deleted-session invalidation")
	host.Shows(t, "Untitled session")
	host.Shows(t, "unsent draft survives forced replacement")
	replacementID := firstRuntimeSession(t, base)
	stop()

	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if draft, found, err := store.Draft("ses_demo_1"); err != nil || found {
		t.Fatalf("deleted session draft = %+v, found %t, error %v", draft, found, err)
	}
	draft, found, err := store.Draft(replacementID)
	if err != nil || !found || draft.Text != "unsent draft survives forced replacement" {
		t.Fatalf("replacement draft = %+v, found %t, error %v", draft, found, err)
	}
}

func TestDeletedSessionReplacementClosesItsQueueEditor(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	backend := &snapshotCountingRuntime{Runtime: base, readSignal: make(chan struct{}, 8)}
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeChanges(t, backend, source, "ses_demo_1")
	host.Shows(t, "Ask lyra")
	awaitSignal(t, source.subscription, "runtime invalidation subscription")

	host.Type("primary before session replacement")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("queued for the deleted session")
	host.Press(input.Enter)
	host.Send(input.Key{Code: input.Character, Rune: 'g', Mods: input.Ctrl})
	host.Shows(t, "Queue · 1 prompt")
	host.Press(input.Enter)
	host.Shows(t, "Editing queued prompt")
	snapshot, err := base.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	active, ok := snapshot.ActiveRun()
	if !ok {
		t.Fatal("queued editor test has no active run")
	}
	if _, err := base.CancelRun(t.Context(), agent.CancelRun{RunID: active.ID}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = base.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	if _, active := snapshot.ActiveRun(); active {
		t.Fatal("canceling the active run did not make the session deletable")
	}

	if err := base.DeleteSession(t.Context(), agent.DeleteSession{SessionID: "ses_demo_1"}); err != nil {
		t.Fatal(err)
	}
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitSignal(t, source.applied, "deleted-session invalidation")
	host.Shows(t, "Untitled session")
	host.Hides(t, "Editing queued prompt")
	host.Hides(t, "Queue · 1 prompt")
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

func TestAdvancedInterruptInvalidationClosesTheApprovalArgumentEditor(t *testing.T) {
	base := mock.New()
	base.Instant = true
	base.Script = func(string) mock.Script {
		return mock.Script{
			Interactions: []agent.Interaction{agent.Approval{
				ItemID: "approval_editor_invalidation", Title: "Run generated command",
				Tool: &agent.ToolCall{
					Kind: agent.ToolShell, Name: "shell", Command: "go test ./...", Status: agent.ToolRunning,
					ArgumentsJSON: []byte(`{"command":"go test ./..."}`),
				},
			}},
			Continue: func([]agent.InterruptAnswer) []mock.Step {
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
	host.Type("test an externally settled approval")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")
	host.Press(input.End)
	host.Press(input.Enter)
	host.Shows(t, "Edit tool arguments")
	host.Type("unsaved editor draft")

	snapshot, err := base.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	active, ok := snapshot.ActiveRun()
	if !ok {
		t.Fatal("approval editor invalidation test has no waiting run")
	}
	if _, err := base.CancelRun(t.Context(), agent.CancelRun{RunID: active.ID, Reason: "settled elsewhere"}); err != nil {
		t.Fatal(err)
	}
	drainSignals(backend.readSignal)
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.InterruptsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitSignal(t, source.applied, "advanced interrupts.changed delivery")
	awaitSignal(t, backend.readSignal, "advanced interrupts.changed authoritative read")
	host.Hides(t, "Edit tool arguments")
	host.Hides(t, "Tool approval")
	host.Shows(t, "canceled")
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
			return nil
		},
		applyResync: func(topics []changefeed.Topic) error {
			resyncs = append(resyncs, slices.Clone(topics))
			if len(resyncs) == 2 {
				cancel()
			}
			return nil
		},
	}
	monitor.run(ctx)
	if len(events) != 1 || events[0].Sequence != 1 {
		t.Fatalf("events = %+v, want only the contiguous frame before the gap", events)
	}
	if len(resyncs) != 2 || !slices.Equal(resyncs[0], resyncs[1]) ||
		!slices.Equal(resyncs[0], []changefeed.Topic{
			changefeed.SessionsChanged, changefeed.RunsChanged,
			changefeed.StateChanged, changefeed.InterruptsChanged,
		}) {
		t.Fatalf("resyncs = %+v", resyncs)
	}
}

func TestRuntimeChangeMonitorDetectsASequenceGapOnTheFirstFrame(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
	}
	source.events <- changefeed.Event{Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 2}
	var resyncs [][]changefeed.Topic
	monitor := runtimeChangeMonitor{
		source: source,
		applyEvent: func(changefeed.Event) error {
			return errors.New("a frame absorbed by gap recovery must not be applied again")
		},
		applyResync: func(topics []changefeed.Topic) error {
			resyncs = append(resyncs, slices.Clone(topics))
			if len(resyncs) == 2 {
				cancel()
			}
			return nil
		},
	}
	monitor.run(ctx)
	if len(resyncs) != 2 || !slices.Equal(resyncs[0], resyncs[1]) {
		t.Fatalf("resyncs = %+v, want matching initial and first-frame-gap reads", resyncs)
	}
}

func TestRuntimeChangeMonitorRefreshesFilesOnceForASequenceGap(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	service := newWorkspaceServiceStub()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		supported: []changefeed.Topic{changefeed.FilesChanged},
	}
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.FilesChanged), Sequence: 2,
		WatchID: workspaceWatchID, Workspace: "/workspace", Paths: []string{"main.go"},
	}
	resyncs := 0
	monitor := runtimeChangeMonitor{
		workspace: "/workspace", repository: service, source: source, watchFiles: true,
		applyFiles: func([]workspace.Change) error { return nil },
		applyResync: func([]changefeed.Topic) error {
			resyncs++
			if resyncs == 2 {
				cancel()
			}
			return nil
		},
	}
	monitor.run(ctx)
	if reads := service.callCount("changes"); reads != 2 {
		t.Fatalf("workspace change reads = %d, want initial read plus one gap recovery", reads)
	}
}

func TestRuntimeChangeMonitorRefreshesFilesForBroadWorkspaceInvalidations(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	service := newWorkspaceServiceStub()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 2), subscription: make(chan changefeed.Subscription, 1),
		supported: []changefeed.Topic{changefeed.FilesChanged},
	}
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.FilesChanged), Sequence: 1,
		Workspace: "/other", Paths: []string{"foreign.go"},
	}
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.FilesChanged), Sequence: 2,
		Workspace: "/workspace", Paths: []string{"main.go"},
	}
	applied := 0
	monitor := runtimeChangeMonitor{
		workspace: "/workspace", repository: service, source: source, watchFiles: true,
		applyFiles: func([]workspace.Change) error {
			applied++
			if applied == 2 {
				cancel()
			}
			return nil
		},
	}
	monitor.run(ctx)
	if reads := service.callCount("changes"); reads != 2 || applied != 2 {
		t.Fatalf("workspace reads/applies = %d/%d, want initial plus current-workspace broad invalidation", reads, applied)
	}
}

func TestRuntimeChangeMonitorTreatsAnUnscopedFileInvalidationAsBroad(t *testing.T) {
	t.Parallel()
	monitor := runtimeChangeMonitor{workspace: "/workspace"}
	tests := []struct {
		name  string
		event changefeed.Event
		want  bool
	}{
		{name: "global broad", event: changefeed.Event{Type: changefeed.EventType(changefeed.FilesChanged)}, want: true},
		{name: "current broad", event: changefeed.Event{Type: changefeed.EventType(changefeed.FilesChanged), Workspace: "/workspace"}, want: true},
		{name: "foreign broad", event: changefeed.Event{Type: changefeed.EventType(changefeed.FilesChanged), Workspace: "/other"}},
		{name: "current watch", event: changefeed.Event{Type: changefeed.EventType(changefeed.FilesChanged), WatchID: workspaceWatchID, Workspace: "/workspace"}, want: true},
		{name: "current watch without workspace", event: changefeed.Event{Type: changefeed.EventType(changefeed.FilesChanged), WatchID: workspaceWatchID}, want: true},
		{name: "current watch wrong workspace", event: changefeed.Event{Type: changefeed.EventType(changefeed.FilesChanged), WatchID: workspaceWatchID, Workspace: "/other"}},
		{name: "foreign watch", event: changefeed.Event{Type: changefeed.EventType(changefeed.FilesChanged), WatchID: "other", Workspace: "/workspace"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := monitor.invalidatesFiles(test.event); got != test.want {
				t.Fatalf("invalidatesFiles(%+v) = %t, want %t", test.event, got, test.want)
			}
		})
	}
}

func TestRuntimeChangeMonitorAppliesAContiguousScopedResync(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		supported: []changefeed.Topic{changefeed.SkillsChanged},
	}
	source.events <- changefeed.Event{
		Type: changefeed.Resync, Sequence: 1, Topics: []changefeed.Topic{changefeed.SkillsChanged},
	}
	var applied []changefeed.Event
	monitor := runtimeChangeMonitor{
		source: source, includeSkills: true,
		applyEvent: func(event changefeed.Event) error {
			applied = append(applied, event)
			cancel()
			return nil
		},
	}
	monitor.run(ctx)
	if len(applied) != 1 || applied[0].Type != changefeed.Resync ||
		!slices.Equal(applied[0].Topics, []changefeed.Topic{changefeed.SkillsChanged}) {
		t.Fatalf("applied events = %+v, want the scoped resync frame", applied)
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
		{name: "plan state", event: changefeed.Event{Type: changefeed.EventType(changefeed.StateChanged), StateKey: changefeed.StatePlan}, want: true},
		{name: "unsupported state", event: changefeed.Event{Type: changefeed.EventType(changefeed.StateChanged), StateKey: "vendor-state"}},
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

func TestRunChangesInvalidateSessionActivityCatalog(t *testing.T) {
	tests := []struct {
		name  string
		event changefeed.Event
		want  bool
	}{
		{name: "session change", event: changefeed.Event{Type: changefeed.EventType(changefeed.SessionsChanged)}, want: true},
		{name: "run change", event: changefeed.Event{Type: changefeed.EventType(changefeed.RunsChanged)}, want: true},
		{name: "run resync", event: changefeed.Event{Type: changefeed.Resync, Topics: []changefeed.Topic{changefeed.RunsChanged}}, want: true},
		{name: "unrelated resync", event: changefeed.Event{Type: changefeed.Resync, Topics: []changefeed.Topic{changefeed.GoalsChanged}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := invalidatesSessionCatalog(test.event); got != test.want {
				t.Fatalf("invalidatesSessionCatalog() = %t, want %t", got, test.want)
			}
		})
	}
}
