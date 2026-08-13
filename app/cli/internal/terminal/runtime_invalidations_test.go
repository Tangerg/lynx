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
	"github.com/Tangerg/lynx/app/cli/internal/codebase"
	"github.com/Tangerg/lynx/app/cli/internal/modelconfig"
	"github.com/Tangerg/lynx/app/cli/internal/retry"
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

type mutableRuntimeCatalog struct {
	agent.Runtime

	mu      sync.Mutex
	models  []agent.Model
	rules   []agent.ApprovalRule
	deleted chan string
}

func (catalog *mutableRuntimeCatalog) ListModels(context.Context) ([]agent.Model, error) {
	catalog.mu.Lock()
	defer catalog.mu.Unlock()
	return slices.Clone(catalog.models), nil
}

func (catalog *mutableRuntimeCatalog) ListApprovalRules(context.Context, string) ([]agent.ApprovalRule, error) {
	catalog.mu.Lock()
	defer catalog.mu.Unlock()
	return slices.Clone(catalog.rules), nil
}

func (catalog *mutableRuntimeCatalog) DeleteApprovalRule(_ context.Context, id string) error {
	catalog.mu.Lock()
	catalog.rules = slices.DeleteFunc(catalog.rules, func(rule agent.ApprovalRule) bool { return rule.ID == id })
	catalog.mu.Unlock()
	if catalog.deleted != nil {
		catalog.deleted <- id
	}
	return nil
}

func (catalog *mutableRuntimeCatalog) setModels(models ...agent.Model) {
	catalog.mu.Lock()
	catalog.models = slices.Clone(models)
	catalog.mu.Unlock()
}

func (catalog *mutableRuntimeCatalog) setRules(rules ...agent.ApprovalRule) {
	catalog.mu.Lock()
	catalog.rules = slices.Clone(rules)
	catalog.mu.Unlock()
}

type mutableCodebaseService struct {
	mu        sync.Mutex
	status    codebase.Status
	hits      []codebase.Hit
	searches  []codebase.Query
	reindexes atomic.Int32
}

func (service *mutableCodebaseService) Status(context.Context, string) (codebase.Status, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.status, nil
}

func (service *mutableCodebaseService) Search(_ context.Context, query codebase.Query) ([]codebase.Hit, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.searches = append(service.searches, query)
	return slices.Clone(service.hits), nil
}

func (service *mutableCodebaseService) Reindex(context.Context, string) (codebase.ReindexOperation, error) {
	service.reindexes.Add(1)
	return codebase.ReindexOperation{ID: "op_test"}, nil
}

func (service *mutableCodebaseService) setHits(hits ...codebase.Hit) {
	service.mu.Lock()
	service.hits = slices.Clone(hits)
	service.mu.Unlock()
}

func (service *mutableCodebaseService) setStatus(status codebase.Status) {
	service.mu.Lock()
	service.status = status
	service.mu.Unlock()
}

func (service *mutableCodebaseService) searchQueries() []codebase.Query {
	service.mu.Lock()
	defer service.mu.Unlock()
	return slices.Clone(service.searches)
}

func TestRuntimeResourceInvalidationsRefreshTheOpenProjection(t *testing.T) {
	t.Run("model picker", func(t *testing.T) {
		catalog := &mutableRuntimeCatalog{Runtime: mock.New()}
		catalog.setModels(agent.Model{ID: "old", Provider: "mock", DisplayName: "Old model"})
		source := runtimeResourceChangeSource(changefeed.ModelsChanged)
		host, stop := runUIWithRuntimeServices(t, Config{Runtime: catalog, Changes: source})
		host.Shows(t, "Ask lyra")
		assertSingleRuntimeTopic(t, source.subscription, changefeed.ModelsChanged)
		host.Type("/model")
		host.Press(input.Enter)
		host.Shows(t, "Old model")

		catalog.setModels(agent.Model{ID: "new", Provider: "mock", DisplayName: "New model"})
		source.events <- changefeed.Event{Type: changefeed.EventType(changefeed.ModelsChanged), Sequence: 1}
		awaitSignal(t, source.applied, "models.changed delivery")
		host.Shows(t, "New model")
		host.Hides(t, "Old model")
		stop()
	})

	t.Run("model reader", func(t *testing.T) {
		catalog := &mutableRuntimeCatalog{Runtime: mock.New()}
		catalog.setModels(agent.Model{ID: "old", Provider: "mock", DisplayName: "Old catalog model"})
		source := runtimeResourceChangeSource(changefeed.ModelsChanged)
		host, stop := runUIWithRuntimeServices(t, Config{Runtime: catalog, Changes: source})
		host.Shows(t, "Ask lyra")
		assertSingleRuntimeTopic(t, source.subscription, changefeed.ModelsChanged)
		host.Type("/models")
		host.Press(input.Enter)
		host.Shows(t, "Old catalog model")

		catalog.setModels(agent.Model{ID: "new", Provider: "mock", DisplayName: "New catalog model"})
		source.events <- changefeed.Event{Type: changefeed.EventType(changefeed.ModelsChanged), Sequence: 1}
		awaitSignal(t, source.applied, "models.changed delivery")
		host.Shows(t, "New catalog model")
		host.Hides(t, "Old catalog model")
		stop()
	})

	t.Run("model roles", func(t *testing.T) {
		models := newModelConfigServiceStub()
		source := runtimeResourceChangeSource(changefeed.ModelsChanged)
		host, stop := runUIWithRuntimeServices(t, Config{
			Runtime: mock.New(), ModelConfig: models, Changes: source,
		})
		host.Shows(t, "Ask lyra")
		assertSingleRuntimeTopic(t, source.subscription, changefeed.ModelsChanged)
		host.Type("/roles")
		host.Press(input.Enter)
		host.Shows(t, "inherit the run model")

		models.mu.Lock()
		models.roles.Utility = modelconfig.Role{Kind: modelconfig.UtilityRole, Provider: "deepseek", Model: "chat"}
		models.mu.Unlock()
		source.events <- changefeed.Event{Type: changefeed.EventType(changefeed.ModelsChanged), Sequence: 1}
		awaitSignal(t, source.applied, "models.changed role delivery")
		host.Shows(t, "deepseek/chat")
		host.Hides(t, "inherit the run model")
		stop()
	})

	t.Run("providers", func(t *testing.T) {
		models := newModelConfigServiceStub()
		source := runtimeResourceChangeSource(changefeed.ModelsChanged)
		host, stop := runUIWithRuntimeServices(t, Config{
			Runtime: mock.New(), ModelConfig: models, Changes: source,
		})
		host.Shows(t, "Ask lyra")
		assertSingleRuntimeTopic(t, source.subscription, changefeed.ModelsChanged)
		host.Type("/providers")
		host.Press(input.Enter)
		host.Shows(t, "api.deepseek.example")

		models.mu.Lock()
		models.providers[0].BaseURL = "https://new.deepseek.example"
		models.mu.Unlock()
		source.events <- changefeed.Event{Type: changefeed.EventType(changefeed.ModelsChanged), Sequence: 1}
		awaitSignal(t, source.applied, "models.changed provider delivery")
		host.Shows(t, "new.deepseek.example")
		host.Hides(t, "api.deepseek.example")
		stop()
	})

	t.Run("approval rules", func(t *testing.T) {
		catalog := &mutableRuntimeCatalog{Runtime: mock.New()}
		source := runtimeResourceChangeSource(changefeed.ApprovalsChanged)
		host, stop := runUIWithRuntimeServices(t, Config{Runtime: catalog, Changes: source})
		host.Shows(t, "Ask lyra")
		assertSingleRuntimeTopic(t, source.subscription, changefeed.ApprovalsChanged)
		host.Type("/rules")
		host.Press(input.Enter)
		host.Shows(t, "No remembered approval rules")

		catalog.setRules(agent.ApprovalRule{
			ID: "rule_external", Scope: agent.RememberGlobal, Tool: "shell",
			Subject: "go test ./...", Decision: agent.ApprovalRuleAllow,
		})
		source.events <- changefeed.Event{Type: changefeed.EventType(changefeed.ApprovalsChanged), Sequence: 1}
		awaitSignal(t, source.applied, "approvals.changed delivery")
		host.Shows(t, "rule_external")
		host.Hides(t, "No remembered approval rules")
		stop()
	})

	t.Run("agent memory", func(t *testing.T) {
		memory := newAgentMemoryServiceStub()
		source := runtimeResourceChangeSource(changefeed.AgentMemoryChanged)
		host, stop := runUIWithRuntimeServices(t, Config{
			Runtime: mock.New(), Workspace: "/workspace", AgentMemory: memory, Changes: source,
		})
		host.Shows(t, "Ask lyra")
		assertSingleRuntimeTopic(t, source.subscription, changefeed.AgentMemoryChanged)
		host.Type("/memory project")
		host.Press(input.Enter)
		host.Shows(t, "confirm release steps")

		memory.mu.Lock()
		memory.project[0].Content = "confirm externally revised release steps"
		memory.project[0].UpdatedAt = time.Now()
		memory.mu.Unlock()
		source.events <- changefeed.Event{Type: changefeed.EventType(changefeed.AgentMemoryChanged), Sequence: 1}
		awaitSignal(t, source.applied, "agentMemory.changed delivery")
		host.Shows(t, "confirm externally revised release steps")
		host.Hides(t, "confirm release steps")
		stop()
	})

	t.Run("codebase search", func(t *testing.T) {
		index := &mutableCodebaseService{status: codebase.Status{State: codebase.Ready}}
		index.setHits(codebase.Hit{
			Path: "internal/old.go", StartLine: 1, EndLine: 2, Snippet: "old owner", Score: .8,
		})
		source := runtimeResourceChangeSource(changefeed.CodebaseChanged)
		host, stop := runUIWithRuntimeServices(t, Config{
			Runtime: mock.New(), Workspace: "/workspace", Codebase: index, Changes: source,
		})
		host.Shows(t, "Ask lyra")
		assertSingleRuntimeTopic(t, source.subscription, changefeed.CodebaseChanged)
		host.Type("/codebase-search ownership")
		host.Press(input.Enter)
		host.Shows(t, "internal/old.go")

		index.setHits(codebase.Hit{
			Path: "internal/new.go", StartLine: 3, EndLine: 5, Snippet: "new owner", Score: .9,
		})
		source.events <- changefeed.Event{Type: changefeed.EventType(changefeed.CodebaseChanged), Sequence: 1}
		awaitSignal(t, source.applied, "codebase.changed delivery")
		host.Shows(t, "internal/new.go")
		host.Hides(t, "internal/old.go")
		queries := index.searchQueries()
		if len(queries) < 2 {
			t.Fatalf("codebase refresh queries = %+v, want at least the initial and invalidation reads", queries)
		}
		for _, query := range queries {
			if query != queries[0] || query.Text != "ownership" {
				t.Fatalf("codebase refresh queries = %+v, want the exact original query", queries)
			}
		}
		stop()
	})

	t.Run("codebase status", func(t *testing.T) {
		index := &mutableCodebaseService{status: codebase.Status{
			State: codebase.Ready, FileCount: 12, ChunkCount: 24,
		}}
		source := runtimeResourceChangeSource(changefeed.CodebaseChanged)
		host, stop := runUIWithRuntimeServices(t, Config{
			Runtime: mock.New(), Workspace: "/workspace", Codebase: index, Changes: source,
		})
		host.Shows(t, "Ask lyra")
		assertSingleRuntimeTopic(t, source.subscription, changefeed.CodebaseChanged)
		host.Type("/codebase")
		host.Press(input.Enter)
		host.Shows(t, "files      12")

		index.setStatus(codebase.Status{State: codebase.Ready, FileCount: 17, ChunkCount: 31})
		source.events <- changefeed.Event{Type: changefeed.EventType(changefeed.CodebaseChanged), Sequence: 1}
		awaitSignal(t, source.applied, "codebase.changed status delivery")
		host.Shows(t, "files      17")
		host.Hides(t, "files      12")
		stop()
	})
}

func TestApprovalRuleDeletionResolvesAUniquePrefixAndSurvivesResize(t *testing.T) {
	catalog := &mutableRuntimeCatalog{Runtime: mock.New(), deleted: make(chan string, 1)}
	catalog.setRules(agent.ApprovalRule{
		ID: "rule_external_123", Scope: agent.RememberGlobal, Tool: "shell",
		Subject: "go test ./...", Decision: agent.ApprovalRuleAllow,
	})
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: catalog})
	host.Shows(t, "Ask lyra")
	host.Type("/rules")
	host.Press(input.Enter)
	host.Shows(t, "rule_external_123")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/rule-delete rule_ext")
	host.Press(input.Enter)
	host.Shows(t, "Forget approval rule")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("approval-rule deletion confirmation did not survive a minimal viewport")
	}
	host.Shows(t, "rule_external_123")
	host.Press(input.Down)
	host.Press(input.Enter)
	if id := awaitValue(t, catalog.deleted, "approval rule deletion"); id != "rule_external_123" {
		t.Fatalf("deleted approval rule = %q", id)
	}
	host.Shows(t, "No remembered approval rules")
	stop()
}

func TestResolveApprovalRuleRequiresAnUnambiguousIdentity(t *testing.T) {
	t.Parallel()
	rules := []agent.ApprovalRule{{ID: "rule_alpha"}, {ID: "rule_alpine"}}
	if rule, err := resolveApprovalRule(rules, "rule_alpha"); err != nil || rule.ID != "rule_alpha" {
		t.Fatalf("exact rule = (%+v, %v)", rule, err)
	}
	if _, err := resolveApprovalRule(rules, "rule_al"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous rule error = %v", err)
	}
	if _, err := resolveApprovalRule(rules, "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing rule error = %v", err)
	}
}

func TestWorkspaceReplacementRetiresThePreviousProjectionConfirmation(t *testing.T) {
	backend := mock.New()
	index := &mutableCodebaseService{}
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: backend, Codebase: index, Changes: source, SessionID: "ses_demo_1",
	})
	host.Shows(t, "Ask lyra")
	awaitValue(t, source.subscription, "runtime invalidation subscription")
	host.Type("/codebase-reindex")
	host.Press(input.Enter)
	host.Shows(t, "Reindex codebase")

	snapshot, err := backend.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	replacementWorkspace := t.TempDir()
	if _, err := backend.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: snapshot.Session.ID, Workspace: &replacementWorkspace,
		ExpectedRevision: snapshot.Session.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitSignal(t, source.applied, "workspace replacement invalidation")
	host.Hides(t, "Reindex codebase")
	host.Press(input.Down)
	host.Press(input.Enter)
	if calls := index.reindexes.Load(); calls != 0 {
		t.Fatalf("retired projection confirmation started %d reindexes", calls)
	}
	stop()
}

func runtimeResourceChangeSource(topic changefeed.Topic) *runtimeChangeSourceStub {
	return &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1), supported: []changefeed.Topic{topic},
	}
}

func assertSingleRuntimeTopic(t *testing.T, subscriptions <-chan changefeed.Subscription, topic changefeed.Topic) {
	t.Helper()
	subscription := awaitValue(t, subscriptions, string(topic)+" subscription")
	if !slices.Equal(subscription.Topics, []changefeed.Topic{topic}) {
		t.Fatalf("runtime subscription topics = %v, want [%s]", subscription.Topics, topic)
	}
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
		recovery: retry.Backoff{Base: 20 * time.Millisecond, Maximum: 40 * time.Millisecond},
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
		source: source, resources: runtimeResourceObservation{plan: true, skills: true},
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
		resources:          runtimeResourceObservation{plan: true},
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
		resources: runtimeResourceObservation{plan: true},
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

func TestRuntimeChangeMonitorPreservesAuthoredWorkspaceScopeAcrossPartitions(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	source := &partitionedRuntimeChangeSourceStub{
		supported: []changefeed.Topic{
			changefeed.FilesChanged, changefeed.SessionsChanged,
			changefeed.KnowledgeChanged, changefeed.HooksChanged,
		},
		registrations: make(chan runtimeSubscriptionRegistration, 3),
	}
	var fileReads atomic.Int32
	fileRefreshes := make(chan struct{}, 2)
	monitor := runtimeChangeMonitor{
		workspace: "/workspace", source: source, watchFiles: true,
		repository: changeReaderFunc(func(context.Context, string) ([]workspace.Change, error) {
			fileReads.Add(1)
			return nil, nil
		}),
		resources: runtimeResourceObservation{knowledge: true, hooks: true},
		subscriptionLimits: changefeed.SubscriptionLimits{
			MaxTopics: 2, MaxWatches: 1,
		},
		applyFiles: func([]workspace.Change) error {
			fileRefreshes <- struct{}{}
			return nil
		},
	}
	done := make(chan error, 1)
	go func() { done <- monitor.run(ctx) }()

	registrations := make([]runtimeSubscriptionRegistration, 0, 3)
	for range 3 {
		registrations = append(registrations, awaitValue(t, source.registrations, "authored-resource partition"))
	}
	for _, topic := range []changefeed.Topic{changefeed.KnowledgeChanged, changefeed.HooksChanged} {
		index := slices.IndexFunc(registrations, func(registration runtimeSubscriptionRegistration) bool {
			return containsTopic(registration.subscription.Topics, topic)
		})
		if index < 0 {
			t.Fatalf("%s partition is missing", topic)
		}
		subscription := registrations[index].subscription
		if !containsTopic(subscription.Topics, changefeed.FilesChanged) ||
			!slices.Equal(subscription.Watches, []changefeed.Watch{{ID: workspaceWatchID, Workspace: "/workspace"}}) {
			t.Fatalf("%s partition lost workspace scope: %+v", topic, subscription)
		}
	}

	awaitSignal(t, fileRefreshes, "owned initial file refresh")
	if fileReads.Load() != 1 {
		t.Fatalf("initial file reads = %d, want one projection owner", fileReads.Load())
	}
	for _, registration := range registrations {
		if containsTopic(registration.subscription.Topics, changefeed.FilesChanged) {
			registration.events <- changefeed.Event{
				Type: changefeed.EventType(changefeed.FilesChanged), Sequence: 1,
				WatchID: workspaceWatchID, Workspace: "/workspace", Paths: []string{"main.go"},
			}
		}
	}
	awaitSignal(t, fileRefreshes, "owned changed-file refresh")
	time.Sleep(25 * time.Millisecond)
	if fileReads.Load() != 2 {
		t.Fatalf("file reads after duplicate partition events = %d, want one owner refresh", fileReads.Load())
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

type blockedResumeRuntime struct {
	agent.Runtime
	started chan agent.ResumeRun
	release chan struct{}
	calls   atomic.Int32
}

func (runtime *blockedResumeRuntime) ResumeRun(ctx context.Context, input agent.ResumeRun) (agent.SegmentStream, error) {
	runtime.calls.Add(1)
	select {
	case runtime.started <- input.Clone():
	case <-ctx.Done():
		return agent.SegmentStream{}, context.Cause(ctx)
	}
	select {
	case <-runtime.release:
		return agent.SegmentStream{}, agent.ErrInterruptNotOpen
	case <-ctx.Done():
		return agent.SegmentStream{}, context.Cause(ctx)
	}
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
	host.Press(input.Down)
	host.Press(input.Tab)
	host.Type("PRESERVED_INVALIDATION_FEEDBACK")
	drainSignals(backend.readSignal)
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.InterruptsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitSignal(t, source.applied, "interrupts.changed delivery")
	awaitSignal(t, backend.readSignal, "interrupts.changed authoritative read")
	host.Shows(t, "Tool approval")
	host.Shows(t, "PRESERVED_INVALIDATION_FEEDBACK")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	provided := <-answers
	if len(provided) != 1 {
		t.Fatalf("approval responses = %+v", provided)
	}
	answer, ok := provided[0].Answer.(agent.ApprovalAnswer)
	if !ok || answer.Decision != agent.ApprovalDeny || answer.Reason != "PRESERVED_INVALIDATION_FEEDBACK" {
		t.Fatalf("preserved approval answer = %#v", provided[0].Answer)
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

func TestInterruptInvalidationWinsARejectedStaleResume(t *testing.T) {
	base := mock.New()
	base.Instant = true
	base.Script = func(string) mock.Script {
		return mock.Script{
			Interactions: []agent.Interaction{agent.Approval{
				ItemID: "approval_resume_race", Title: "Run generated command",
				Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Command: "go test ./...", Status: agent.ToolRunning},
			}},
			Continue: func([]agent.InterruptAnswer) []mock.Step {
				return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	backend := &snapshotCountingRuntime{Runtime: base, readSignal: make(chan struct{}, 8)}
	runtime := &blockedResumeRuntime{
		Runtime: backend, started: make(chan agent.ResumeRun, 1), release: make(chan struct{}),
	}
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeChanges(t, runtime, source, "ses_demo_1")
	host.Shows(t, "Ask lyra")
	awaitSignal(t, source.subscription, "runtime invalidation subscription")
	host.Type("exercise the approval race")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")
	host.Press(input.Enter)
	pending := awaitSignalValue(t, runtime.started, "blocked resume delivery")

	continued, err := base.ResumeRun(t.Context(), pending)
	if err != nil {
		t.Fatal(err)
	}
	for _, streamErr := range continued.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
	}
	drainSignals(backend.readSignal)
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.InterruptsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitSignal(t, source.applied, "interrupts.changed during resume delivery")
	close(runtime.release)
	awaitSignal(t, backend.readSignal, "authoritative read after stale resume refusal")
	host.Shows(t, "session refreshed after runtime change")
	host.Shows(t, "done")
	host.Hides(t, "Tool approval")
	if calls := runtime.calls.Load(); calls != 1 {
		t.Fatalf("resume delivery calls = %d, want no stale retry", calls)
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

func awaitSignalValue[T any](t *testing.T, signals <-chan T, what string) T {
	t.Helper()
	select {
	case value := <-signals:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for " + what)
		var zero T
		return zero
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
		source: source, resources: runtimeResourceObservation{plan: true},
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
		source: source, resources: runtimeResourceObservation{plan: true},
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
		source: source, resources: runtimeResourceObservation{skills: true},
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
