package runtimeembedded

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/goal"
	"github.com/Tangerg/lynx/app/cli/internal/modelconfig"
)

type usageBindingStub struct {
	session func(context.Context, protocol.SessionUsageRequest, embedded.CallOptions) (*protocol.Usage, error)
	summary func(context.Context, protocol.UsageSummaryRequest, embedded.CallOptions) (*protocol.UsageSummary, error)
}

func (stub usageBindingStub) GetSessionUsage(ctx context.Context, request protocol.SessionUsageRequest, options embedded.CallOptions) (*protocol.Usage, error) {
	return stub.session(ctx, request, options)
}

func (stub usageBindingStub) GetUsageSummary(ctx context.Context, request protocol.UsageSummaryRequest, options embedded.CallOptions) (*protocol.UsageSummary, error) {
	return stub.summary(ctx, request, options)
}

func TestUsageAdapterProjectsSessionAndSummaryReports(t *testing.T) {
	cost := 0.25
	stub := usageBindingStub{
		session: func(_ context.Context, request protocol.SessionUsageRequest, options embedded.CallOptions) (*protocol.Usage, error) {
			if request.SessionID != "ses_1" || options.RequestMeta.ProtocolVersion != protocol.ProtocolVersion {
				t.Fatalf("session usage request = %+v, options = %+v", request, options)
			}
			return &protocol.Usage{
				ModelUsage: protocol.ModelUsage{InputTokens: 12, CostUSD: &cost},
				ByModel: map[string]protocol.ModelUsage{
					"z/model": {OutputTokens: 2}, "a/model": {InputTokens: 3},
				},
			}, nil
		},
		summary: func(_ context.Context, request protocol.UsageSummaryRequest, _ embedded.CallOptions) (*protocol.UsageSummary, error) {
			if request.SinceDays != 30 {
				t.Fatalf("summary request = %+v", request)
			}
			return &protocol.UsageSummary{
				Total: protocol.ModelUsage{InputTokens: 20}, Sessions: 2, Runs: 4,
				ByProvider: []protocol.UsageBucket{{Key: "deepseek", Runs: 4}},
			}, nil
		},
	}
	runtime := &Runtime{usage: stub, meta: requestMeta("test")}
	session, err := runtime.SessionUsage(t.Context(), "ses_1")
	if err != nil || len(session.ByModel) != 2 || session.ByModel[0].Key != "a/model" || session.Total.CostUSD == nil {
		t.Fatalf("SessionUsage = (%+v, %v)", session, err)
	}
	summary, err := runtime.Summary(t.Context(), 30)
	if err != nil || summary.Runs != 4 || len(summary.ByProvider) != 1 {
		t.Fatalf("Summary = (%+v, %v)", summary, err)
	}
}

func TestUsageAdapterRejectsInvalidRuntimeReports(t *testing.T) {
	t.Parallel()
	runtime := &Runtime{usage: usageBindingStub{
		session: func(context.Context, protocol.SessionUsageRequest, embedded.CallOptions) (*protocol.Usage, error) {
			return &protocol.Usage{ModelUsage: protocol.ModelUsage{InputTokens: -1}}, nil
		},
		summary: func(context.Context, protocol.UsageSummaryRequest, embedded.CallOptions) (*protocol.UsageSummary, error) {
			return &protocol.UsageSummary{ByModel: []protocol.UsageBucket{{Key: "same"}, {Key: "same"}}}, nil
		},
	}, meta: requestMeta("test")}
	_, err := runtime.SessionUsage(t.Context(), "ses_1")
	requireRuntimeContractViolation(t, err)
	_, err = runtime.Summary(t.Context(), 7)
	requireRuntimeContractViolation(t, err)
}

type modelConfigBindingStub struct {
	utility       protocol.UtilityRole
	embedding     protocol.EmbeddingRole
	providers     []protocol.Provider
	utilityReply  *protocol.UtilityRole
	providerReply *protocol.Provider
	utilitySet    func(protocol.UtilityRole, embedded.CommandOptions)
	embeddingSet  func(protocol.EmbeddingRole, embedded.CommandOptions)
	updated       func(protocol.UpdateProviderRequest, embedded.CommandOptions)
}

func (stub *modelConfigBindingStub) GetUtilityRole(context.Context, embedded.CallOptions) (*protocol.UtilityRole, error) {
	return &stub.utility, nil
}

func (stub *modelConfigBindingStub) SetUtilityRole(_ context.Context, request protocol.UtilityRole, options embedded.CommandOptions) (*protocol.UtilityRole, error) {
	stub.utilitySet(request, options)
	if stub.utilityReply != nil {
		return stub.utilityReply, nil
	}
	stub.utility = request
	return &stub.utility, nil
}

func (stub *modelConfigBindingStub) GetEmbeddingRole(context.Context, embedded.CallOptions) (*protocol.EmbeddingRole, error) {
	return &stub.embedding, nil
}

func (stub *modelConfigBindingStub) SetEmbeddingRole(_ context.Context, request protocol.EmbeddingRole, options embedded.CommandOptions) (*protocol.EmbeddingRole, error) {
	stub.embeddingSet(request, options)
	stub.embedding = request
	return &stub.embedding, nil
}

func (stub *modelConfigBindingStub) ListProviders(context.Context, embedded.CallOptions) (*protocol.Page[protocol.Provider], error) {
	return protocol.NewPage(stub.providers), nil
}

func (stub *modelConfigBindingStub) UpdateProvider(_ context.Context, request protocol.UpdateProviderRequest, options embedded.CommandOptions) (*protocol.Provider, error) {
	stub.updated(request, options)
	if stub.providerReply != nil {
		return stub.providerReply, nil
	}
	return &stub.providers[0], nil
}

func TestModelConfigurationRejectsMutationIdentityDrift(t *testing.T) {
	t.Parallel()
	stub := &modelConfigBindingStub{
		utilityReply:  &protocol.UtilityRole{Provider: "other", Model: "model"},
		providerReply: &protocol.Provider{ID: "other"},
		utilitySet:    func(protocol.UtilityRole, embedded.CommandOptions) {},
		updated:       func(protocol.UpdateProviderRequest, embedded.CommandOptions) {},
	}
	runtime := &Runtime{modelConfig: stub, meta: requestMeta("test")}
	_, err := runtime.SetRole(t.Context(), modelconfig.Role{Kind: modelconfig.UtilityRole, Provider: "deepseek", Model: "chat"})
	requireRuntimeContractViolation(t, err)
	change := modelconfig.ValueChange{Kind: modelconfig.ClearValue}
	_, err = runtime.UpdateProvider(t.Context(), modelconfig.UpdateProvider{Provider: "deepseek", APIKey: &change})
	requireRuntimeContractViolation(t, err)
}

func (*modelConfigBindingStub) TestProvider(_ context.Context, request protocol.TestProviderRequest, _ embedded.CallOptions) (*protocol.ProviderTestResult, error) {
	return &protocol.ProviderTestResult{OK: false, Error: &protocol.ProblemData{
		Type: protocol.ProblemProviderUnavailable, Detail: request.Provider,
		DocURL: "https://docs.example/providers", RetryAfterSeconds: 3,
	}}, nil
}

func TestModelConfigurationAdapterPreservesRoleAndSecretMutationSemantics(t *testing.T) {
	stub := &modelConfigBindingStub{
		utility: protocol.UtilityRole{Provider: "deepseek", Model: "chat"},
		providers: []protocol.Provider{{
			ID: "deepseek", APIKeyMasked: "sk****42", KeySource: protocol.ProviderKeySourceStored,
			EmbeddingCapable: true,
		}},
	}
	assertCommand := func(options embedded.CommandOptions) {
		if options.IdempotencyKey == "" || options.RequestMeta.ProtocolVersion != protocol.ProtocolVersion {
			t.Fatalf("command options = %+v", options)
		}
	}
	stub.utilitySet = func(request protocol.UtilityRole, options embedded.CommandOptions) {
		assertCommand(options)
		if request.Provider != "openai" || request.Model != "utility" {
			t.Fatalf("utility role request = %+v", request)
		}
	}
	stub.embeddingSet = func(request protocol.EmbeddingRole, options embedded.CommandOptions) {
		assertCommand(options)
		if request != (protocol.EmbeddingRole{}) {
			t.Fatalf("embedding role request = %+v", request)
		}
	}
	stub.updated = func(request protocol.UpdateProviderRequest, options embedded.CommandOptions) {
		assertCommand(options)
		if request.APIKey == nil || request.APIKey.Type != protocol.ProviderConfigSet || request.APIKey.Value == nil || *request.APIKey.Value != "secret" {
			t.Fatalf("provider update request = %+v", request)
		}
		if request.BaseURL == nil || request.BaseURL.Type != protocol.ProviderConfigClear || request.BaseURL.Value != nil {
			t.Fatalf("provider endpoint update = %+v", request.BaseURL)
		}
	}
	runtime := &Runtime{modelConfig: stub, meta: requestMeta("test")}
	roles, err := runtime.Roles(t.Context())
	if err != nil || roles.Utility.Label() != "deepseek/chat" || roles.Embedding.Configured() {
		t.Fatalf("Roles = (%+v, %v)", roles, err)
	}
	if _, err := runtime.SetRole(t.Context(), modelconfig.Role{Kind: modelconfig.UtilityRole, Provider: "openai", Model: "utility"}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SetRole(t.Context(), modelconfig.Role{Kind: modelconfig.EmbeddingRole}); err != nil {
		t.Fatal(err)
	}
	providers, err := runtime.Providers(t.Context())
	if err != nil || len(providers) != 1 || !providers[0].Configured() {
		t.Fatalf("Providers = (%+v, %v)", providers, err)
	}
	secret := modelconfig.ValueChange{Kind: modelconfig.SetValue, Value: "secret"}
	clear := modelconfig.ValueChange{Kind: modelconfig.ClearValue}
	if _, err := runtime.UpdateProvider(t.Context(), modelconfig.UpdateProvider{Provider: "deepseek", BaseURL: &clear, APIKey: &secret}); err != nil {
		t.Fatal(err)
	}
	tested, err := runtime.TestProvider(t.Context(), "deepseek")
	if err != nil || tested.OK || tested.Problem == nil || tested.Problem.String() != "provider_unavailable: deepseek · retry after 3s · docs https://docs.example/providers" {
		t.Fatalf("TestProvider = (%+v, %v)", tested, err)
	}
}

type goalBindingStub struct {
	t       *testing.T
	current *protocol.Goal
	last    string
}

func (stub *goalBindingStub) GetGoal(context.Context, protocol.GoalRequest, embedded.CallOptions) (*protocol.Goal, error) {
	return stub.current, nil
}

func (stub *goalBindingStub) StartGoal(_ context.Context, request protocol.StartGoalRequest, options embedded.CommandOptions) (*protocol.Goal, error) {
	if request.SessionID != "ses_1" || request.Objective != "finish" || request.Budget.MaxRuns != 3 || options.IdempotencyKey == "" {
		stub.t.Fatalf("start goal request = %+v, options = %+v", request, options)
	}
	stub.last = "start"
	stub.current = activeProtocolGoal()
	return stub.current, nil
}

func (stub *goalBindingStub) StopGoal(context.Context, protocol.GoalRequest, embedded.CommandOptions) (*protocol.Goal, error) {
	stub.last = "stop"
	stopped := *stub.current
	stopped.Status = protocol.GoalPaused
	stopped.Reason = &protocol.GoalReason{Code: protocol.GoalReasonStoppedByUser}
	stub.current = &stopped
	return stub.current, nil
}

func (stub *goalBindingStub) ResumeGoal(context.Context, protocol.GoalRequest, embedded.CommandOptions) (*protocol.Goal, error) {
	stub.last = "resume"
	resumed := *stub.current
	resumed.Status = protocol.GoalActive
	resumed.Reason = nil
	stub.current = &resumed
	return stub.current, nil
}

func activeProtocolGoal() *protocol.Goal {
	return &protocol.Goal{
		SessionID: "ses_1", Objective: "finish", Status: protocol.GoalActive,
		Budget: protocol.GoalBudget{MaxRuns: 3}, CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(2, 0),
	}
}

func TestGoalAdapterProjectsTheCompleteLifecycle(t *testing.T) {
	stub := &goalBindingStub{t: t}
	runtime := &Runtime{goals: stub, meta: requestMeta("test")}
	if _, exists, err := runtime.GetGoal(t.Context(), "ses_1"); err != nil || exists {
		t.Fatalf("empty GetGoal = (%t, %v)", exists, err)
	}
	started, err := runtime.StartGoal(t.Context(), goal.Start{SessionID: "ses_1", Objective: "finish", Budget: goal.Budget{MaxRuns: 3}})
	if err != nil || started.Status != goal.Active || stub.last != "start" {
		t.Fatalf("StartGoal = (%+v, %v), last %q", started, err, stub.last)
	}
	stopped, err := runtime.StopGoal(t.Context(), "ses_1")
	if err != nil || stopped.Status != goal.Paused || stopped.Reason == nil || stub.last != "stop" {
		t.Fatalf("StopGoal = (%+v, %v), last %q", stopped, err, stub.last)
	}
	resumed, err := runtime.ResumeGoal(t.Context(), "ses_1")
	if err != nil || resumed.Status != goal.Active || stub.last != "resume" {
		t.Fatalf("ResumeGoal = (%+v, %v), last %q", resumed, err, stub.last)
	}
	completing := *stub.current
	completing.Status = protocol.GoalCompleting
	stub.current = &completing
	observed, exists, err := runtime.GetGoal(t.Context(), "ses_1")
	if err != nil || !exists || observed.Status != goal.Completing || observed.Reason != nil {
		t.Fatalf("completing GetGoal = (%+v, %t, %v)", observed, exists, err)
	}
}

func TestGoalAdapterRejectsAResponseForAnotherSession(t *testing.T) {
	t.Parallel()
	stub := &goalBindingStub{t: t, current: activeProtocolGoal()}
	runtime := &Runtime{goals: stub, meta: requestMeta("test")}

	_, _, err := runtime.GetGoal(t.Context(), "ses_other")
	requireRuntimeContractViolation(t, err)
}
