package mock

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

func (r *Runtime) ListApprovalRules(ctx context.Context) ([]client.ApprovalRule, error) {
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
	at := slices.IndexFunc(r.rules, func(rule client.ApprovalRule) bool { return rule.ID == id })
	if at < 0 {
		return fmt.Errorf("mock: approval rule %s not found", id)
	}
	r.rules = slices.Delete(r.rules, at, at+1)
	return nil
}

func (r *Runtime) rememberApprovalLocked(run *runState, approval client.Approval, answer client.ApprovalAnswer) {
	session := r.sessions[run.sessionID]
	key := approvalRuleKey(approval)
	for _, rule := range r.rules {
		if rule.Rule == key && rule.Scope == answer.Remember && ruleApplies(rule, run.sessionID, session.meta.Workspace) {
			return
		}
	}
	r.next++
	rule := client.ApprovalRule{
		ID: fmt.Sprintf("rule_mock_%d", r.next), Rule: key, Decision: answer.Decision,
		Scope: answer.Remember, CreatedAt: r.now(),
	}
	switch answer.Remember {
	case client.RememberSession:
		rule.SessionID = run.sessionID
	case client.RememberProject:
		rule.Workspace = session.meta.Workspace
	case client.RememberGlobal:
		// Global rules intentionally carry no qualifier.
	case client.RememberNone:
		return
	default:
		return
	}
	r.rules = append(r.rules, rule)
}

func (r *Runtime) rememberedAnswerLocked(run *runState, approval client.Approval) (client.ApprovalAnswer, bool) {
	workspace := r.sessions[run.sessionID].meta.Workspace
	key := approvalRuleKey(approval)
	for _, rule := range slices.Backward(r.rules) {
		if rule.Rule == key && ruleApplies(rule, run.sessionID, workspace) {
			return client.ApprovalAnswer{Decision: rule.Decision, Remember: rule.Scope}, true
		}
	}
	return client.ApprovalAnswer{}, false
}

func ruleApplies(rule client.ApprovalRule, sessionID, workspace string) bool {
	switch rule.Scope {
	case client.RememberSession:
		return rule.SessionID == sessionID
	case client.RememberProject:
		return rule.Workspace == workspace
	case client.RememberGlobal:
		return true
	case client.RememberNone:
		return false
	default:
		return false
	}
}

func approvalRuleKey(approval client.Approval) string {
	if key := strings.TrimSpace(approval.RuleHint); key != "" {
		return key
	}
	return strings.TrimSpace(approval.Title)
}
