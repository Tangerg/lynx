package agent

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

func (m Model) Validate() error {
	var problems []error
	if strings.TrimSpace(m.ID) == "" {
		problems = append(problems, errors.New("id is empty"))
	}
	if strings.TrimSpace(m.Provider) == "" {
		problems = append(problems, errors.New("provider is empty"))
	}
	if m.ContextWindow < 0 || m.MaxInputTokens < 0 || m.MaxOutputTokens < 0 {
		problems = append(problems, errors.New("token limits cannot be negative"))
	}
	if m.KnowledgeCutoff != "" {
		if _, err := time.Parse(time.DateOnly, m.KnowledgeCutoff); err != nil {
			problems = append(problems, fmt.Errorf("knowledge cutoff %q is not an RFC3339 date", m.KnowledgeCutoff))
		}
	}
	if m.Capabilities != nil {
		if err := m.Capabilities.validate(); err != nil {
			problems = append(problems, err)
		}
	}
	if m.Pricing != nil {
		if err := m.Pricing.validate(); err != nil {
			problems = append(problems, err)
		}
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("model: %w", err)
	}
	return nil
}

// Clone returns a model with no capability slices shared with the caller.
func (m Model) Clone() Model {
	if m.Capabilities != nil {
		capabilities := m.Capabilities.clone()
		m.Capabilities = &capabilities
	}
	if m.Pricing != nil {
		pricing := *m.Pricing
		m.Pricing = &pricing
	}
	return m
}

func (capabilities ModelCapabilities) clone() ModelCapabilities {
	capabilities.ReasoningLevels = slices.Clone(capabilities.ReasoningLevels)
	capabilities.InputModalities = slices.Clone(capabilities.InputModalities)
	capabilities.OutputModalities = slices.Clone(capabilities.OutputModalities)
	return capabilities
}

func (capabilities ModelCapabilities) validate() error {
	var problems []error
	if err := validateUniqueModelStrings("reasoning level", capabilities.ReasoningLevels); err != nil {
		problems = append(problems, err)
	}
	if !capabilities.Reasoning && (len(capabilities.ReasoningLevels) != 0 || capabilities.ReasoningDefaultLevel != "") {
		problems = append(problems, errors.New("non-reasoning model carries reasoning configuration"))
	}
	if capabilities.ReasoningDefaultLevel != "" && !slices.Contains(capabilities.ReasoningLevels, capabilities.ReasoningDefaultLevel) {
		problems = append(problems, fmt.Errorf("default reasoning level %q is not offered", capabilities.ReasoningDefaultLevel))
	}
	if err := validateUniqueModalities("input", capabilities.InputModalities); err != nil {
		problems = append(problems, err)
	}
	if err := validateUniqueModalities("output", capabilities.OutputModalities); err != nil {
		problems = append(problems, err)
	}
	return errors.Join(problems...)
}

func validateUniqueModelStrings(label string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is empty", label)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s %q is duplicated", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateUniqueModalities(direction string, modalities []ModelModality) error {
	seen := make(map[ModelModality]struct{}, len(modalities))
	for _, modality := range modalities {
		if strings.TrimSpace(string(modality)) == "" {
			return fmt.Errorf("%s modality is empty", direction)
		}
		if _, duplicate := seen[modality]; duplicate {
			return fmt.Errorf("%s modality %q is duplicated", direction, modality)
		}
		seen[modality] = struct{}{}
	}
	return nil
}

func (pricing ModelPricing) validate() error {
	rates := []float64{
		pricing.InputUSDPerMillionTokens,
		pricing.OutputUSDPerMillionTokens,
		pricing.CacheReadUSDPerMillionTokens,
		pricing.CacheWriteUSDPerMillionTokens,
	}
	for _, rate := range rates {
		if rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
			return errors.New("pricing rates must be finite and non-negative")
		}
	}
	return nil
}

func ValidateModels(models []Model) error {
	identities := make(map[string]struct{}, len(models))
	for i, model := range models {
		if err := model.Validate(); err != nil {
			return fmt.Errorf("model %d: %w", i+1, err)
		}
		identity := model.Provider + "\x00" + model.ID
		if _, duplicate := identities[identity]; duplicate {
			return fmt.Errorf("model %s/%s is duplicated", model.Provider, model.ID)
		}
		identities[identity] = struct{}{}
	}
	return nil
}

func (m ApprovalMode) Validate() error {
	if !slices.Contains([]ApprovalMode{ApprovalModeSafe, ApprovalModeBalanced, ApprovalModeYolo}, m) {
		return fmt.Errorf("approval mode %q is invalid", m)
	}
	return nil
}

func (r ApprovalRule) Validate() error {
	if strings.TrimSpace(r.ID) == "" || strings.TrimSpace(r.Tool) == "" {
		return errors.New("approval rule: id and tool are required")
	}
	if !slices.Contains([]ApprovalRuleDecision{ApprovalRuleAllow, ApprovalRuleDeny}, r.Decision) {
		return fmt.Errorf("approval rule: decision %q is invalid", r.Decision)
	}
	if !slices.Contains([]RememberScope{RememberSession, RememberProject, RememberGlobal}, r.Scope) {
		return fmt.Errorf("approval rule: scope %q is invalid", r.Scope)
	}
	if r.Scope == RememberProject && strings.TrimSpace(r.Dir) == "" {
		return errors.New("approval rule: project scope requires a directory")
	}
	if r.Scope != RememberProject && r.Dir != "" {
		return errors.New("approval rule: only project scope may carry a directory")
	}
	return nil
}

func ValidateApprovalRules(rules []ApprovalRule) error {
	ids := make(map[string]struct{}, len(rules))
	for i, rule := range rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("approval rule %d: %w", i+1, err)
		}
		if _, duplicate := ids[rule.ID]; duplicate {
			return fmt.Errorf("approval rule id %q is duplicated", rule.ID)
		}
		ids[rule.ID] = struct{}{}
	}
	return nil
}
