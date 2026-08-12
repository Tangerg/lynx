package runtimeembedded

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
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
	for _, provider := range providerValues {
		page, err := r.modelCatalog.ListModels(ctx, protocol.ListModelsRequest{Provider: provider.ID}, r.callOptions())
		if err != nil {
			return nil, classifyError(err)
		}
		values, err := requireCompletePage("list models for "+provider.ID, page)
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			models = append(models, projectModel(value))
		}
	}
	if err := agent.ValidateModels(models); err != nil {
		return nil, fmt.Errorf("list models projection: %w", err)
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
		return "", errors.New("get approval mode: runtime returned nil")
	}
	mode := agent.ApprovalMode(result.Mode)
	if err := mode.Validate(); err != nil {
		return "", fmt.Errorf("get approval mode projection: %w", err)
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
		return "", errors.New("set approval mode: runtime returned nil")
	}
	applied := agent.ApprovalMode(result.Mode)
	if err := applied.Validate(); err != nil {
		return "", fmt.Errorf("set approval mode projection: %w", err)
	}
	return applied, nil
}

func (r *Runtime) ListApprovalRules(ctx context.Context, sessionID string) ([]agent.ApprovalRule, error) {
	result, err := r.approvals.ListApprovalRules(ctx, protocol.ListApprovalRulesRequest{SessionID: sessionID}, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if result == nil {
		return nil, errors.New("list approval rules: runtime returned nil")
	}
	rules := make([]agent.ApprovalRule, 0, len(result.Rules))
	for _, value := range result.Rules {
		rules = append(rules, agent.ApprovalRule{
			ID: value.ID, Scope: agent.RememberScope(value.Scope), Tool: value.Tool,
			Subject: value.Subject, Dir: value.Dir, Decision: agent.ApprovalRuleDecision(value.Decision),
		})
	}
	if err := agent.ValidateApprovalRules(rules); err != nil {
		return nil, fmt.Errorf("list approval rules projection: %w", err)
	}
	return rules, nil
}

func (r *Runtime) DeleteApprovalRule(ctx context.Context, id string) error {
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(r.approvals.ForgetApprovalRule(ctx, protocol.ForgetApprovalRuleRequest{ID: id}, options))
}
