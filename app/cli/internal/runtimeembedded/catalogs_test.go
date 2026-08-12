package runtimeembedded

import (
	"context"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type approvalBindingRecorder struct {
	listRequest   protocol.ListApprovalRulesRequest
	forgetRequest protocol.ForgetApprovalRuleRequest
	forgetOptions embedded.CommandOptions
	listCalls     int
	forgetCalls   int
	setMode       protocol.ApprovalMode
}

func (*approvalBindingRecorder) GetApprovalMode(context.Context, embedded.CallOptions) (*protocol.ApprovalModeResult, error) {
	return &protocol.ApprovalModeResult{Mode: protocol.ApprovalModeBalanced}, nil
}

func (recorder *approvalBindingRecorder) SetApprovalMode(_ context.Context, request protocol.SetApprovalModeRequest, _ embedded.CommandOptions) (*protocol.ApprovalModeResult, error) {
	if recorder.setMode != "" {
		return &protocol.ApprovalModeResult{Mode: recorder.setMode}, nil
	}
	return &protocol.ApprovalModeResult{Mode: request.Mode}, nil
}

func (recorder *approvalBindingRecorder) ListApprovalRules(_ context.Context, request protocol.ListApprovalRulesRequest, _ embedded.CallOptions) (*protocol.ListApprovalRulesResult, error) {
	recorder.listCalls++
	recorder.listRequest = request
	return &protocol.ListApprovalRulesResult{Rules: []protocol.ApprovalRule{{
		ID: "rule_1", Scope: protocol.ApprovalRuleScopeProject, Tool: "shell",
		Subject: "go test *", Dir: "/workspace", Decision: protocol.ApprovalRuleDecisionAllow,
	}}}, nil
}

func TestCatalogsRejectResponsesOutsideTheRequestedIdentity(t *testing.T) {
	t.Parallel()
	models := &Runtime{modelCatalog: modelCatalogBindingStub{
		providers: protocol.NewPage([]protocol.Provider{{ID: "deepseek"}}),
		models: map[string]*protocol.Page[protocol.Model]{
			"deepseek": protocol.NewPage([]protocol.Model{{ID: "chat", Provider: "other"}}),
		},
	}, meta: requestMeta("test")}
	_, err := models.ListModels(t.Context())
	requireRuntimeContractViolation(t, err)

	approvals := &Runtime{approvals: &approvalBindingRecorder{setMode: protocol.ApprovalModeYolo}, meta: requestMeta("test")}
	_, err = approvals.SetApprovalMode(t.Context(), agent.ApprovalModeSafe)
	requireRuntimeContractViolation(t, err)
}

func (recorder *approvalBindingRecorder) ForgetApprovalRule(_ context.Context, request protocol.ForgetApprovalRuleRequest, options embedded.CommandOptions) error {
	recorder.forgetCalls++
	recorder.forgetRequest = request
	recorder.forgetOptions = options
	return nil
}

func TestModelCatalogProjectsEveryPublishedModelField(t *testing.T) {
	t.Parallel()

	capabilities := &protocol.ModelCapabilities{
		Reasoning: true, ReasoningLevels: []string{"low", "high"}, ReasoningDefaultLevel: "high",
		Multimodal: true, InputModalities: []protocol.Modality{protocol.ModalityText, protocol.ModalityImage},
		OutputModalities: []protocol.Modality{protocol.ModalityText}, ToolUse: true, StructuredOutput: true,
	}
	pricing := &protocol.ModelPricing{
		InputUSDPerMillionTokens: 0.2, OutputUSDPerMillionTokens: 0.8,
		CacheReadUSDPerMillionTokens: 0.02, CacheWriteUSDPerMillionTokens: 0.1,
	}
	stub := modelCatalogBindingStub{
		providers: protocol.NewPage([]protocol.Provider{{ID: "provider"}}),
		models: map[string]*protocol.Page[protocol.Model]{
			"provider": protocol.NewPage([]protocol.Model{{
				ID: "reasoner", Provider: "provider", DisplayName: "Reasoner",
				ContextWindow: 200_000, MaxInputTokens: 180_000, MaxOutputTokens: 20_000,
				KnowledgeCutoff: "2026-01-31", Deprecated: true,
				Capabilities: capabilities, Pricing: pricing,
			}}),
		},
	}
	runtime := &Runtime{modelCatalog: stub, meta: requestMeta("test")}
	models, err := runtime.ListModels(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("models = %+v", models)
	}
	model := models[0]
	wantInput := []agent.ModelModality{agent.ModelModalityText, agent.ModelModalityImage}
	if model.ID != "reasoner" || model.Provider != "provider" || model.DisplayName != "Reasoner" ||
		model.ContextWindow != 200_000 || model.MaxInputTokens != 180_000 || model.MaxOutputTokens != 20_000 ||
		model.KnowledgeCutoff != "2026-01-31" || !model.Deprecated || model.Capabilities == nil || model.Pricing == nil ||
		!model.Capabilities.Reasoning || model.Capabilities.ReasoningDefaultLevel != "high" ||
		!model.Capabilities.Multimodal || !model.Capabilities.ToolUse || !model.Capabilities.StructuredOutput ||
		!reflect.DeepEqual(model.Capabilities.InputModalities, wantInput) ||
		model.Pricing.CacheWriteUSDPerMillionTokens != 0.1 {
		t.Fatalf("projected model = %+v", model)
	}
	capabilities.ReasoningLevels[0] = "mutated"
	capabilities.InputModalities[0] = protocol.ModalityAudio
	pricing.InputUSDPerMillionTokens = 99
	if model.Capabilities.ReasoningLevels[0] != "low" ||
		model.Capabilities.InputModalities[0] != agent.ModelModalityText ||
		model.Pricing.InputUSDPerMillionTokens != 0.2 {
		t.Fatal("model projection aliases runtime-owned metadata")
	}
}

func TestApprovalCatalogNormalizesIdentitiesBeforeCrossingTheRuntimeBoundary(t *testing.T) {
	t.Parallel()

	recorder := &approvalBindingRecorder{}
	runtime := &Runtime{approvals: recorder, meta: requestMeta("test")}
	rules, err := runtime.ListApprovalRules(t.Context(), "  session_1  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].ID != "rule_1" || rules[0].Scope != agent.RememberProject ||
		rules[0].Subject != "go test *" || rules[0].Dir != "/workspace" || rules[0].Decision != agent.ApprovalRuleAllow {
		t.Fatalf("approval rules = %+v", rules)
	}
	if recorder.listRequest.SessionID != "session_1" {
		t.Fatalf("list request = %+v", recorder.listRequest)
	}
	if err := runtime.DeleteApprovalRule(t.Context(), "  rule_1  "); err != nil {
		t.Fatal(err)
	}
	if recorder.forgetRequest.ID != "rule_1" || recorder.forgetOptions.IdempotencyKey == "" || recorder.forgetCalls != 1 {
		t.Fatalf("forget request = %+v, options = %+v, calls = %d", recorder.forgetRequest, recorder.forgetOptions, recorder.forgetCalls)
	}

	if _, err := runtime.ListApprovalRules(t.Context(), "  "); err == nil {
		t.Fatal("empty session identity crossed the approval boundary")
	}
	if err := runtime.DeleteApprovalRule(t.Context(), "\t"); err == nil {
		t.Fatal("empty rule identity crossed the approval boundary")
	}
	if recorder.listCalls != 1 || recorder.forgetCalls != 1 {
		t.Fatalf("invalid identities reached runtime: list=%d forget=%d", recorder.listCalls, recorder.forgetCalls)
	}
}
