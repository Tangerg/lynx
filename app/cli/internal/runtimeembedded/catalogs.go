package runtimeembedded

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func (r *Runtime) ListModels(ctx context.Context) ([]agent.Model, error) {
	providers, err := r.binding.ListProviders(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if providers == nil {
		return nil, errors.New("list providers: runtime returned a nil page")
	}

	var models []agent.Model
	for _, provider := range providers.Data {
		page, err := r.binding.ListModels(ctx, protocol.ListModelsRequest{Provider: provider.ID}, r.callOptions())
		if err != nil {
			return nil, classifyError(err)
		}
		if page == nil {
			return nil, fmt.Errorf("list models for %s: runtime returned a nil page", provider.ID)
		}
		for _, value := range page.Data {
			model := agent.Model{
				ID: value.ID, Provider: value.Provider, DisplayName: value.DisplayName,
				ContextWindow: value.ContextWindow, MaxInputTokens: value.MaxInputTokens,
				MaxOutputTokens: value.MaxOutputTokens, Deprecated: value.Deprecated,
			}
			if value.Capabilities != nil {
				model.Capabilities = agent.ModelCapabilities{
					Reasoning: value.Capabilities.Reasoning, ReasoningLevels: slices.Clone(value.Capabilities.ReasoningLevels),
					Multimodal: value.Capabilities.Multimodal, ToolUse: value.Capabilities.ToolUse,
				}
			}
			models = append(models, model)
		}
	}
	if err := agent.ValidateModels(models); err != nil {
		return nil, fmt.Errorf("list models projection: %w", err)
	}
	return models, nil
}

func (r *Runtime) GetApprovalMode(ctx context.Context) (agent.ApprovalMode, error) {
	result, err := r.binding.GetApprovalMode(ctx, r.callOptions())
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
	result, err := r.binding.SetApprovalMode(ctx, protocol.SetApprovalModeRequest{Mode: protocol.ApprovalMode(mode)}, options)
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
	result, err := r.binding.ListApprovalRules(ctx, protocol.ListApprovalRulesRequest{SessionID: sessionID}, r.callOptions())
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
	return classifyError(r.binding.ForgetApprovalRule(ctx, protocol.ForgetApprovalRuleRequest{ID: id}, options))
}
