package client

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Validate checks a model projection before it reaches a picker or run request.
func (m Model) Validate() error {
	var problems []error
	if strings.TrimSpace(m.ID) == "" {
		problems = append(problems, errors.New("id is empty"))
	}
	if strings.TrimSpace(m.DisplayName) == "" {
		problems = append(problems, errors.New("display name is empty"))
	}
	if m.Context < 0 {
		problems = append(problems, errors.New("context size is negative"))
	}
	seen := make(map[string]struct{}, len(m.Efforts))
	for _, effort := range m.Efforts {
		if err := (RunOptions{Effort: effort}).Validate(); err != nil {
			problems = append(problems, err)
		}
		if _, duplicate := seen[effort]; duplicate {
			problems = append(problems, fmt.Errorf("effort %q is duplicated", effort))
		}
		seen[effort] = struct{}{}
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("model: %w", err)
	}
	return nil
}

// ValidateModels checks per-model invariants, unique identities, and a single
// default selection.
func ValidateModels(models []Model) error {
	ids := make(map[string]struct{}, len(models))
	defaults := 0
	for i, model := range models {
		if err := model.Validate(); err != nil {
			return fmt.Errorf("model %d: %w", i+1, err)
		}
		if _, duplicate := ids[model.ID]; duplicate {
			return fmt.Errorf("model id %q is duplicated", model.ID)
		}
		ids[model.ID] = struct{}{}
		if model.Default {
			defaults++
		}
	}
	if defaults > 1 {
		return errors.New("model catalog has more than one default")
	}
	return nil
}

// Validate checks a remembered approval projection and its scope qualifier.
func (r ApprovalRule) Validate() error {
	if strings.TrimSpace(r.ID) == "" || strings.TrimSpace(r.Rule) == "" {
		return errors.New("approval rule: id and rule are required")
	}
	if !slices.Contains([]ApprovalDecision{ApprovalAllow, ApprovalDeny}, r.Decision) {
		return fmt.Errorf("approval rule: decision %q is invalid", r.Decision)
	}
	if !slices.Contains([]RememberScope{RememberSession, RememberProject, RememberGlobal}, r.Scope) {
		return fmt.Errorf("approval rule: scope %q is invalid", r.Scope)
	}
	switch r.Scope {
	case RememberSession:
		if strings.TrimSpace(r.SessionID) == "" || r.Workspace != "" {
			return errors.New("approval rule: session scope requires only a session id")
		}
	case RememberProject:
		if strings.TrimSpace(r.Workspace) == "" || r.SessionID != "" {
			return errors.New("approval rule: project scope requires only a workspace")
		}
	case RememberGlobal:
		if r.SessionID != "" || r.Workspace != "" {
			return errors.New("approval rule: global scope cannot carry a qualifier")
		}
	}
	return nil
}

// ValidateApprovalRules checks rule invariants and unique identities.
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
