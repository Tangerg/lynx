package mock

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/scope/app/cli/internal/agent"
)

func (r *Runtime) ListApprovalRules(ctx context.Context, sessionID string) ([]agent.ApprovalRule, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("list approval rules: session id is empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	session := r.sessions[sessionID]
	if session == nil {
		return nil, fmt.Errorf("%w: %s", agent.ErrSessionNotFound, sessionID)
	}
	out := make([]agent.ApprovalRule, 0, len(r.rules))
	for _, stored := range r.rules {
		if ruleApplies(stored, sessionID, session.meta.Workspace.ProjectRoot) {
			out = append(out, stored.view)
		}
	}
	slices.Reverse(out)
	return out, nil
}

func (r *Runtime) DeleteApprovalRule(ctx context.Context, id string) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("delete approval rule: id is empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	at := slices.IndexFunc(r.rules, func(rule storedRule) bool { return rule.view.ID == id })
	if at >= 0 {
		r.rules = slices.Delete(r.rules, at, at+1)
	}
	return nil
}

func (r *Runtime) rememberApprovalLocked(run *runState, approval agent.Approval, answer agent.ApprovalAnswer) {
	if !approval.Rememberable || answer.Remember == agent.RememberNone {
		return
	}
	session := r.sessions[run.sessionID]
	tool, subject := approvalRuleParts(approval)
	for _, stored := range r.rules {
		rule := stored.view
		if rule.Tool == tool && rule.Subject == subject && rule.Scope == answer.Remember && ruleApplies(stored, run.sessionID, session.meta.Workspace.ProjectRoot) {
			return
		}
	}
	r.next++
	rule := agent.ApprovalRule{
		ID: fmt.Sprintf("rule_mock_%d", r.next), Scope: answer.Remember,
		Tool: tool, Subject: subject, Decision: approvalRuleDecision(answer.Decision),
	}
	stored := storedRule{view: rule}
	switch answer.Remember {
	case agent.RememberSession:
		stored.sessionID = run.sessionID
	case agent.RememberProject:
		stored.view.Dir = session.meta.Workspace.ProjectRoot
	case agent.RememberGlobal:
	default:
		return
	}
	r.rules = append(r.rules, stored)
}

func (r *Runtime) resolveRememberedLocked(run *runState, interactions []agent.Interaction) (resolved []agent.InterruptAnswer, pending []agent.Interaction) {
	resolved = make([]agent.InterruptAnswer, 0, len(interactions))
	pending = make([]agent.Interaction, 0, len(interactions))
	for _, interaction := range interactions {
		approval, ok := interaction.(agent.Approval)
		if !ok {
			pending = append(pending, agent.CloneInteraction(interaction))
			continue
		}
		answer, matched := r.rememberedAnswerLocked(run, approval)
		if !matched {
			pending = append(pending, agent.CloneInteraction(interaction))
			continue
		}
		resolved = append(resolved, agent.InterruptAnswer{ItemID: approval.ItemID, Answer: answer})
	}
	return resolved, pending
}

func (r *Runtime) rememberedAnswerLocked(run *runState, approval agent.Approval) (agent.ApprovalAnswer, bool) {
	workspace := r.sessions[run.sessionID].meta.Workspace.ProjectRoot
	tool, subject := approvalRuleParts(approval)
	for _, stored := range slices.Backward(r.rules) {
		rule := stored.view
		if rule.Tool == tool && rule.Subject == subject && ruleApplies(stored, run.sessionID, workspace) {
			return agent.ApprovalAnswer{Decision: approvalDecision(rule.Decision), Remember: rule.Scope}, true
		}
	}
	return agent.ApprovalAnswer{}, false
}

func approvalRuleDecision(decision agent.ApprovalDecision) agent.ApprovalRuleDecision {
	if decision == agent.ApprovalApprove {
		return agent.ApprovalRuleAllow
	}
	return agent.ApprovalRuleDeny
}

func approvalDecision(decision agent.ApprovalRuleDecision) agent.ApprovalDecision {
	if decision == agent.ApprovalRuleAllow {
		return agent.ApprovalApprove
	}
	return agent.ApprovalDeny
}

func ruleApplies(rule storedRule, sessionID, workspace string) bool {
	switch rule.view.Scope {
	case agent.RememberSession:
		return rule.sessionID == sessionID
	case agent.RememberProject:
		return rule.view.Dir == workspace
	case agent.RememberGlobal:
		return true
	default:
		return false
	}
}

func approvalRuleParts(approval agent.Approval) (tool, subject string) {
	hint := strings.TrimSpace(approval.RuleHint)
	if hint != "" {
		if hintTool, hintSubject, ok := strings.Cut(hint, ":"); ok && strings.TrimSpace(hintTool) != "" {
			return strings.TrimSpace(hintTool), strings.TrimSpace(hintSubject)
		}
	}
	if approval.Tool == nil {
		return "unknown", strings.TrimSpace(approval.Title)
	}
	tool = strings.TrimSpace(approval.Tool.Name)
	if tool == "" {
		tool = string(approval.Tool.Kind)
	}
	switch approval.Tool.Kind {
	case agent.ToolShell:
		subject = approval.Tool.Command
	case agent.ToolEdit, agent.ToolRead:
		subject = approval.Tool.Path
	case agent.ToolSearch:
		subject = approval.Tool.Query
	case agent.ToolWeb:
		subject = approval.Tool.URL
	case agent.ToolUnknown, agent.ToolTask:
	}
	if strings.TrimSpace(subject) == "" {
		subject = approval.Tool.Summary
	}
	return tool, strings.TrimSpace(subject)
}
