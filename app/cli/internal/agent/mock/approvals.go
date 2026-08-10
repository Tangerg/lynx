package mock

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func (r *Runtime) ListApprovalRules(ctx context.Context) ([]agent.ApprovalRule, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := slices.Clone(r.rules)
	slices.Reverse(out)
	return out, nil
}

func (r *Runtime) DeleteApprovalRule(ctx context.Context, id string) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	at := slices.IndexFunc(r.rules, func(rule agent.ApprovalRule) bool { return rule.ID == id })
	if at < 0 {
		return fmt.Errorf("mock: approval rule %s not found", id)
	}
	r.rules = slices.Delete(r.rules, at, at+1)
	return nil
}

func (r *Runtime) rememberApprovalLocked(run *runState, approval agent.Approval, answer agent.ApprovalAnswer) {
	session := r.sessions[run.sessionID]
	key := approvalRuleKey(approval)
	for _, rule := range r.rules {
		if rule.Rule == key && rule.Scope == answer.Remember && ruleApplies(rule, run.sessionID, session.meta.Workspace) {
			return
		}
	}
	r.next++
	rule := agent.ApprovalRule{
		ID: fmt.Sprintf("rule_mock_%d", r.next), Rule: key, Decision: answer.Decision,
		Scope: answer.Remember, CreatedAt: r.now(),
	}
	switch answer.Remember {
	case agent.RememberSession:
		rule.SessionID = run.sessionID
	case agent.RememberProject:
		rule.Workspace = session.meta.Workspace
	case agent.RememberGlobal:
		// Global rules intentionally carry no qualifier.
	case agent.RememberNone:
		return
	default:
		return
	}
	r.rules = append(r.rules, rule)
}

func (r *Runtime) rememberedAnswerLocked(run *runState, approval agent.Approval) (agent.ApprovalAnswer, bool) {
	workspace := r.sessions[run.sessionID].meta.Workspace
	key := approvalRuleKey(approval)
	for _, rule := range slices.Backward(r.rules) {
		if rule.Rule == key && ruleApplies(rule, run.sessionID, workspace) {
			return agent.ApprovalAnswer{Decision: rule.Decision, Remember: rule.Scope}, true
		}
	}
	return agent.ApprovalAnswer{}, false
}

func ruleApplies(rule agent.ApprovalRule, sessionID, workspace string) bool {
	switch rule.Scope {
	case agent.RememberSession:
		return rule.SessionID == sessionID
	case agent.RememberProject:
		return rule.Workspace == workspace
	case agent.RememberGlobal:
		return true
	case agent.RememberNone:
		return false
	default:
		return false
	}
}

func approvalRuleKey(approval agent.Approval) string {
	if key := strings.TrimSpace(approval.RuleHint); key != "" {
		return key
	}
	return strings.TrimSpace(approval.Title)
}
