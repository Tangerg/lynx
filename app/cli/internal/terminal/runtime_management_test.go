package terminal

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/programtest"

	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/goal"
	"github.com/Tangerg/lynx/app/cli/internal/modelconfig"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
	"github.com/Tangerg/lynx/app/cli/internal/usage"
)

type usageServiceStub struct{}

func (usageServiceStub) SessionUsage(_ context.Context, sessionID string) (usage.SessionReport, error) {
	cost := 0.25
	return usage.SessionReport{
		SessionID: sessionID, Total: usage.Totals{InputTokens: 1_200, OutputTokens: 300, CostUSD: &cost},
		ByModel: []usage.Bucket{{Key: "deepseek/model", Totals: usage.Totals{InputTokens: 1_200}}},
	}, nil
}

func (usageServiceStub) Summary(_ context.Context, sinceDays int) (usage.Summary, error) {
	cost := 1.5
	return usage.Summary{
		SinceDays: sinceDays, Total: usage.Totals{InputTokens: 8_000, OutputTokens: 2_000, CostUSD: &cost},
		ByProvider: []usage.Bucket{{Key: "deepseek", Runs: 4}}, Sessions: 2, Runs: 4,
	}, nil
}

type blockingUsageService struct {
	started  chan struct{}
	canceled chan struct{}
}

func (service blockingUsageService) SessionUsage(ctx context.Context, _ string) (usage.SessionReport, error) {
	close(service.started)
	<-ctx.Done()
	close(service.canceled)
	return usage.SessionReport{}, context.Cause(ctx)
}

func (blockingUsageService) Summary(context.Context, int) (usage.Summary, error) {
	panic("summary must not run after the session usage query is canceled")
}

func TestUsageAndModelRoleCommandsProjectRuntimeConfiguration(t *testing.T) {
	models := newModelConfigServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: mock.New(), Usage: usageServiceStub{}, ModelConfig: models,
	})
	host.Shows(t, "Ask lyra")
	host.Type("/usage 30")
	host.Press(input.Enter)
	host.Shows(t, "Runtime usage")
	host.Shows(t, "last 30 days · 2 sessions · 4 runs")
	host.Shows(t, "input 1,200")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")

	host.Type("/roles")
	host.Press(input.Enter)
	host.Shows(t, "Auxiliary model roles")
	host.Shows(t, "inherit the run model")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/utility deepseek/maintenance")
	host.Press(input.Enter)
	host.Shows(t, "utility model · deepseek/maintenance")
	host.Type("/embedding off")
	host.Press(input.Enter)
	host.Shows(t, "embedding model · disabled")
	roles, err := models.Roles(t.Context())
	if err != nil || roles.Utility.Model != "maintenance" || roles.Embedding.Configured() {
		t.Fatalf("roles = (%+v, %v)", roles, err)
	}
	stop()
}

func TestRuntimeStatusConsumesTheNegotiatedDiscoveryProfile(t *testing.T) {
	profile := runtimeprofile.Profile{
		Protocol:  runtimeprofile.Protocol{Current: "2.0", MinSupported: "2.0"},
		Server:    runtimeprofile.Server{Name: "lyra-runtime", Version: "1.2.3", DefaultWorkspace: "/workspace", Home: "/home/test"},
		RunEvents: []string{"segment.started"}, RuntimeTopics: []string{"files.changed"},
		StateSnapshots:   []runtimeprofile.Snapshot{{Key: "plan", RecoveryMethod: "plan.get", Scope: "session", Writer: "rootRun"}},
		StreamingMethods: []string{"runs.start"},
		Features: map[runtimeprofile.FeatureName]runtimeprofile.Feature{
			runtimeprofile.FeatureMCP: {Enabled: true, Stability: runtimeprofile.Stable},
		},
		Limits: runtimeprofile.Limits{
			MaxConcurrentRuns: 4, IdempotencyRetentionSeconds: 600,
			RunReplay:                        runtimeprofile.ReplayLimits{Scope: "runtimeInstanceRootSegment", MaxEvents: 1024, MaxBytes: 1 << 20},
			MCPAuthorizationRetentionSeconds: 600,
			RuntimeSubscription:              runtimeprofile.SubscriptionLimits{MaxTopics: 16, MaxWatches: 32},
		},
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), RuntimeProfile: &profile})
	host.Shows(t, "Ask lyra")
	host.Type("/status")
	host.Press(input.Enter)
	for _, want := range []string{
		"lyra-runtime 1.2.3", "protocol: 2.0", "default workspace: /workspace", "available features: mcp",
		"run concurrency: 4 runs", "run replay: 1024 events / 1 MiB", "command replay retention: 10m",
		"runtime subscriptions: 16 topics / 32 watches", "1 run events / 1 topics / 1 snapshots / 1 streaming methods",
	} {
		host.Shows(t, want)
	}
	stop()
}

func TestSessionReplacementCancelsAnOutstandingSideQuery(t *testing.T) {
	usageService := blockingUsageService{started: make(chan struct{}), canceled: make(chan struct{})}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), Usage: usageService})
	host.Shows(t, "Ask lyra")
	host.Type("/usage")
	host.Press(input.Enter)
	awaitSignal(t, usageService.started, "session usage query")

	host.Type("/new")
	host.Press(input.Enter)
	awaitSignal(t, usageService.canceled, "old session query cancellation")
	host.Hides(t, "Runtime usage")
	stop()
}

type modelConfigServiceStub struct {
	mu        sync.Mutex
	roles     modelconfig.Roles
	providers []modelconfig.Provider
	updates   chan modelconfig.UpdateProvider
}

func newModelConfigServiceStub() *modelConfigServiceStub {
	return &modelConfigServiceStub{
		roles: modelconfig.Roles{
			Utility:   modelconfig.Role{Kind: modelconfig.UtilityRole},
			Embedding: modelconfig.Role{Kind: modelconfig.EmbeddingRole},
		},
		providers: []modelconfig.Provider{{
			ID: "deepseek", BaseURL: "https://api.deepseek.example", APIKeyMasked: "sk****42",
			KeySource: modelconfig.KeyStored,
		}},
		updates: make(chan modelconfig.UpdateProvider, 1),
	}
}

func (service *modelConfigServiceStub) Roles(context.Context) (modelconfig.Roles, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.roles, nil
}

func (service *modelConfigServiceStub) SetRole(_ context.Context, role modelconfig.Role) (modelconfig.Role, error) {
	if err := role.Validate(); err != nil {
		return modelconfig.Role{}, err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if role.Kind == modelconfig.UtilityRole {
		service.roles.Utility = role
	} else {
		service.roles.Embedding = role
	}
	return role, nil
}

func (service *modelConfigServiceStub) Providers(context.Context) ([]modelconfig.Provider, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	return append([]modelconfig.Provider(nil), service.providers...), nil
}

func (service *modelConfigServiceStub) UpdateProvider(_ context.Context, update modelconfig.UpdateProvider) (modelconfig.Provider, error) {
	if err := update.Validate(); err != nil {
		return modelconfig.Provider{}, err
	}
	cloned := update
	if update.BaseURL != nil {
		value := *update.BaseURL
		cloned.BaseURL = &value
	}
	if update.APIKey != nil {
		value := *update.APIKey
		cloned.APIKey = &value
	}
	service.updates <- cloned
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.providers[0], nil
}

func (*modelConfigServiceStub) TestProvider(_ context.Context, providerID string) (modelconfig.TestResult, error) {
	if providerID == "deepseek" {
		return modelconfig.TestResult{OK: true}, nil
	}
	return modelconfig.TestResult{}, errors.New("unknown provider")
}

func TestProviderConfigurationMasksSecretsAndPreservesExplicitChanges(t *testing.T) {
	models := newModelConfigServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), ModelConfig: models})
	host.Shows(t, "Ask lyra")
	host.Type("/providers")
	host.Press(input.Enter)
	host.Shows(t, "sk****42")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/provider-test deepseek")
	host.Press(input.Enter)
	host.Shows(t, "provider deepseek is reachable")

	host.Type("/provider-config deepseek")
	host.Press(input.Enter)
	host.Shows(t, "Configure provider · deepseek")
	host.Press(input.Tab)
	host.Press(input.Tab)
	host.Press(input.Down)
	host.Press(input.Tab)
	secret := "SECRET_PROVIDER_KEY_42"
	host.Type(secret)
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("provider configuration did not survive a minimal viewport")
	}
	host.Shows(t, "Configure provider · deepseek")
	if strings.Contains(host.Frames(), secret) {
		t.Fatal("masked provider key appeared in terminal output")
	}
	host.Press(input.Enter)
	host.Shows(t, "provider updated · deepseek")
	update := <-models.updates
	if update.BaseURL != nil || update.APIKey == nil || update.APIKey.Kind != modelconfig.SetValue || update.APIKey.Value != secret {
		t.Fatalf("provider update = %+v", update)
	}
	stop()
}

type goalServiceStub struct {
	mu      sync.Mutex
	current *goal.Goal
	reads   atomic.Int32
}

func (service *goalServiceStub) GetGoal(context.Context, string) (goal.Goal, bool, error) {
	service.reads.Add(1)
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.current == nil {
		return goal.Goal{}, false, nil
	}
	current := *service.current
	if current.Reason != nil {
		current.Reason = new(*current.Reason)
	}
	return current, true, nil
}

func (service *goalServiceStub) StartGoal(_ context.Context, start goal.Start) (goal.Goal, error) {
	if err := start.Validate(); err != nil {
		return goal.Goal{}, err
	}
	current := goal.Goal{
		SessionID: start.SessionID, Objective: start.Objective, Status: goal.Active,
		Provider: start.Provider, Model: start.Model, Budget: start.Budget,
	}
	service.set(current)
	return current, nil
}

func (service *goalServiceStub) StopGoal(context.Context, string) (goal.Goal, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.current == nil {
		return goal.Goal{}, errors.New("no goal")
	}
	service.current.Status = goal.Paused
	service.current.Reason = &goal.Reason{Code: goal.StoppedByUser}
	return *service.current, nil
}

func (service *goalServiceStub) ResumeGoal(context.Context, string) (goal.Goal, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.current == nil {
		return goal.Goal{}, errors.New("no goal")
	}
	service.current.Status = goal.Active
	service.current.Reason = nil
	return *service.current, nil
}

func (service *goalServiceStub) set(current goal.Goal) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.current = &current
}

func TestGoalLifecycleAndInvalidationRefreshTheOpenGoalReader(t *testing.T) {
	goals := new(goalServiceStub)
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 2), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 2), supported: []changefeed.Topic{changefeed.GoalsChanged},
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), Goals: goals, Changes: source})
	host.Shows(t, "Ask lyra")
	subscription := awaitValue(t, source.subscription, "goal invalidation subscription")
	if len(subscription.Topics) != 1 || subscription.Topics[0] != changefeed.GoalsChanged {
		t.Fatalf("goal subscription = %+v", subscription)
	}
	host.Type("/goal")
	host.Press(input.Enter)
	host.Shows(t, "No autonomous goal")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/goal-start finish the release")
	host.Press(input.Enter)
	host.Shows(t, "finish the release")
	host.Shows(t, "active")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/goal-stop")
	host.Press(input.Enter)
	host.Shows(t, "stoppedByUser")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/goal-resume")
	host.Press(input.Enter)
	host.Shows(t, "active")

	current, exists, err := goals.GetGoal(t.Context(), "ses_demo_1")
	if err != nil || !exists {
		t.Fatalf("goal = (%+v, %t, %v)", current, exists, err)
	}
	current.Status = goal.Blocked
	current.Reason = &goal.Reason{Code: goal.BlockedByModel, Detail: "needs clarification"}
	goals.set(current)
	baseline := goals.reads.Load()
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.GoalsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitSignal(t, source.applied, "goals.changed delivery")
	host.Shows(t, "needs clarification")
	if goals.reads.Load() <= baseline {
		t.Fatal("goals.changed did not refetch the goal")
	}
	current.Status = goal.Completing
	current.Reason = nil
	goals.set(current)
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.GoalsChanged), Sequence: 2,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitSignal(t, source.applied, "completing goals.changed delivery")
	host.Shows(t, "completing")
	stop()
}

func runUIWithRuntimeServices(t *testing.T, config Config) (*programtest.Host, func()) {
	t.Helper()
	if config.SessionID == "" && config.Workspace == "" {
		config.SessionID = "ses_demo_1"
	}
	host := programtest.New(t, 96, 28)
	config.Host = host
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, config) }()
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

func awaitValue[T any](t *testing.T, values <-chan T, what string) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for " + what)
		return *new(T)
	}
}
