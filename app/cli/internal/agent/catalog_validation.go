package agent

import (
	"errors"
	"fmt"
	"slices"
	"strings"
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
	seen := make(map[string]struct{}, len(m.Capabilities.ReasoningLevels))
	for _, level := range m.Capabilities.ReasoningLevels {
		if strings.TrimSpace(level) == "" {
			problems = append(problems, errors.New("reasoning level is empty"))
		}
		if _, duplicate := seen[level]; duplicate {
			problems = append(problems, fmt.Errorf("reasoning level %q is duplicated", level))
		}
		seen[level] = struct{}{}
	}
	if !m.Capabilities.Reasoning && len(m.Capabilities.ReasoningLevels) != 0 {
		problems = append(problems, errors.New("non-reasoning model carries reasoning levels"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("model: %w", err)
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
