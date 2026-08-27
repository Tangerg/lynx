package runtimeembedded

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/Tangerg/scope/app/runtime/embedded"
	"github.com/Tangerg/scope/app/runtime/protocol"

	"github.com/Tangerg/scope/app/cli/internal/agent"
)

type modelCatalogBinding interface {
	ListProviders(context.Context, embedded.CallOptions) (*protocol.Page[protocol.Provider], error)
	ListModels(context.Context, protocol.ListModelsRequest, embedded.CallOptions) (*protocol.Page[protocol.Model], error)
}

type approvalBinding interface {
	GetApprovalMode(context.Context, embedded.CallOptions) (*protocol.ApprovalModeResult, error)
	SetApprovalMode(context.Context, protocol.SetApprovalModeRequest, embedded.CommandOptions) (*protocol.ApprovalModeResult, error)
	ListApprovalRules(context.Context, protocol.ListApprovalRulesRequest, embedded.CallOptions) (*protocol.ListApprovalRulesResult, error)
	ForgetApprovalRule(context.Context, protocol.ForgetApprovalRuleRequest, embedded.CommandOptions) error
}

func (r *Runtime) ListModels(ctx context.Context) ([]agent.Model, error) {
	providers, err := r.modelCatalog.ListProviders(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	providerValues, err := requireCompletePage("list providers", providers)
	if err != nil {
		return nil, err
	}

	var models []agent.Model
	seenProviders := make(map[string]struct{}, len(providerValues))
	for _, provider := range providerValues {
		projectedProvider := projectProvider(provider)
		if err := projectedProvider.Validate(); err != nil {
			return nil, runtimeContractViolation("model catalog returned an invalid provider: %v", err)
		}
		if _, duplicate := seenProviders[provider.ID]; duplicate {
			return nil, runtimeContractViolation("model catalog repeats provider %q", provider.ID)
		}
		seenProviders[provider.ID] = struct{}{}
		page, err := r.modelCatalog.ListModels(ctx, protocol.ListModelsRequest{Provider: provider.ID}, r.callOptions())
		if err != nil {
			return nil, classifyError(err)
		}
		values, err := requireCompletePage("list models for "+provider.ID, page)
		if err != nil {
			return nil, err
		}
		for index, value := range values {
			if value.Provider != provider.ID {
				return nil, runtimeContractViolation("models for provider %q returned model %q from %q", provider.ID, value.ID, value.Provider)
			}
			projected := projectModel(value)
			if err := projected.Validate(); err != nil {
				return nil, runtimeContractViolation("models for provider %q returned invalid item %d: %v", provider.ID, index+1, err)
			}
			models = append(models, projected)
		}
	}
	if err := agent.ValidateModels(models); err != nil {
		return nil, runtimeContractViolation("list models returned an invalid projection: %v", err)
	}
	return models, nil
}

func projectModel(value protocol.Model) agent.Model {
	model := agent.Model{
		ID: value.ID, Provider: value.Provider, DisplayName: value.DisplayName,
		ContextWindow: value.ContextWindow, MaxInputTokens: value.MaxInputTokens,
		MaxOutputTokens: value.MaxOutputTokens, KnowledgeCutoff: value.KnowledgeCutoff,
		Deprecated: value.Deprecated,
	}
	if value.Capabilities != nil {
		capabilities := &agent.ModelCapabilities{
			Reasoning: value.Capabilities.Reasoning, ReasoningLevels: slices.Clone(value.Capabilities.ReasoningLevels),
			ReasoningDefaultLevel: value.Capabilities.ReasoningDefaultLevel,
			Multimodal:            value.Capabilities.Multimodal,
			ToolUse:               value.Capabilities.ToolUse,
			StructuredOutput:      value.Capabilities.StructuredOutput,
			InputModalities:       make([]agent.ModelModality, len(value.Capabilities.InputModalities)),
			OutputModalities:      make([]agent.ModelModality, len(value.Capabilities.OutputModalities)),
		}
		for index, modality := range value.Capabilities.InputModalities {
			capabilities.InputModalities[index] = agent.ModelModality(modality)
		}
		for index, modality := range value.Capabilities.OutputModalities {
			capabilities.OutputModalities[index] = agent.ModelModality(modality)
		}
		model.Capabilities = capabilities
	}
	if value.Pricing != nil {
		model.Pricing = &agent.ModelPricing{
			InputUSDPerMillionTokens:      value.Pricing.InputUSDPerMillionTokens,
			OutputUSDPerMillionTokens:     value.Pricing.OutputUSDPerMillionTokens,
			CacheReadUSDPerMillionTokens:  value.Pricing.CacheReadUSDPerMillionTokens,
			CacheWriteUSDPerMillionTokens: value.Pricing.CacheWriteUSDPerMillionTokens,
		}
	}
	return model
}

func (r *Runtime) GetApprovalMode(ctx context.Context) (agent.ApprovalMode, error) {
	result, err := r.approvals.GetApprovalMode(ctx, r.callOptions())
	if err != nil {
		return "", classifyError(err)
	}
	if result == nil {
		return "", runtimeContractViolation("get approval mode returned nil")
	}
	mode := agent.ApprovalMode(result.Mode)
	if err := mode.Validate(); err != nil {
		return "", runtimeContractViolation("get approval mode returned an invalid projection: %v", err)
	}
	return mode, nil
}

func (r *Runtime) SetApprovalMode(ctx context.Context, mode agent.ApprovalMode) (agent.ApprovalMode, error) {
	if err := mode.Validate(); err != nil {
		return "", err
	}
	options, err := r.commandOptions()
	if err != nil {
		return "", err
	}
	result, err := r.approvals.SetApprovalMode(ctx, protocol.SetApprovalModeRequest{Mode: protocol.ApprovalMode(mode)}, options)
	if err != nil {
		return "", classifyError(err)
	}
	if result == nil {
		return "", runtimeContractViolation("set approval mode returned nil")
	}
	applied := agent.ApprovalMode(result.Mode)
	if err := applied.Validate(); err != nil {
		return "", runtimeContractViolation("set approval mode returned an invalid projection: %v", err)
	}
	if applied != mode {
		return "", runtimeContractViolation("set approval mode returned %q for %q", applied, mode)
	}
	return applied, nil
}

func (r *Runtime) ListApprovalRules(ctx context.Context, sessionID string) ([]agent.ApprovalRule, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("list approval rules: session id is empty")
	}
	result, err := r.approvals.ListApprovalRules(ctx, protocol.ListApprovalRulesRequest{SessionID: sessionID}, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if result == nil {
		return nil, runtimeContractViolation("list approval rules returned nil")
	}
	rules := make([]agent.ApprovalRule, 0, len(result.Rules))
	for _, value := range result.Rules {
		rules = append(rules, agent.ApprovalRule{
			ID: value.ID, Scope: agent.RememberScope(value.Scope), Tool: value.Tool,
			Subject: value.Subject, Dir: value.Dir, Decision: agent.ApprovalRuleDecision(value.Decision),
		})
	}
	if err := agent.ValidateApprovalRules(rules); err != nil {
		return nil, runtimeContractViolation("list approval rules returned an invalid projection: %v", err)
	}
	return rules, nil
}

func (r *Runtime) DeleteApprovalRule(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("delete approval rule: id is empty")
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(r.approvals.ForgetApprovalRule(ctx, protocol.ForgetApprovalRuleRequest{ID: id}, options))
}
