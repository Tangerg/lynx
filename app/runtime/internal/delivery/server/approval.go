package server

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// GetApprovalMode returns the runtime's default tool-permission stance
// (approval.getMode).
func (s *Server) GetApprovalMode(ctx context.Context) (*protocol.ApprovalModeResult, error) {
	m, err := s.approvals.DefaultMode(ctx)
	if err != nil {
		return nil, err
	}
	mode, ok := presentApprovalMode(m)
	if !ok {
		return nil, fmt.Errorf("server: %w: %q", approval.ErrInvalidMode, m)
	}
	return &protocol.ApprovalModeResult{Mode: mode}, nil
}

// SetApprovalMode sets the runtime's default tool-permission stance (approval.setMode).
func (s *Server) SetApprovalMode(ctx context.Context, in protocol.SetApprovalModeRequest) (*protocol.ApprovalModeResult, error) {
	mode, ok := approvalModeFromWire(in.Mode)
	if !ok {
		return nil, fmt.Errorf("%w: unknown approval mode %q", protocol.ErrInvalidParams, in.Mode)
	}
	if err := s.approvals.SetDefaultMode(ctx, mode); err != nil {
		return nil, err
	}
	return &protocol.ApprovalModeResult{Mode: in.Mode}, nil
}

// ListApprovalRules lists the persisted rules visible from a session
// (approval.listRules) — its session rules, its project's rules, and all
// global rules. Runtime resolves the session's project directory; an unknown
// session degrades to session + global only.
func (s *Server) ListApprovalRules(ctx context.Context, in protocol.ListApprovalRulesRequest) (*protocol.ListApprovalRulesResult, error) {
	rules, err := s.approvals.ListRules(ctx, in.SessionID)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.ApprovalRule, 0, len(rules))
	for _, r := range rules {
		wire, err := presentApprovalRule(r)
		if err != nil {
			return nil, err
		}
		out = append(out, wire)
	}
	return &protocol.ListApprovalRulesResult{Rules: out}, nil
}

// ForgetApprovalRule removes one persisted approval rule by id
// (approval.forgetRule). A missing id is not an error.
func (s *Server) ForgetApprovalRule(ctx context.Context, in protocol.ForgetApprovalRuleRequest) error {
	return s.approvals.ForgetRule(ctx, in.ID)
}

// presentApprovalRule maps a domain rule to its protocol shape. The project
// directory is surfaced only for project-scoped rules (the UI shows where they
// apply); session/global rules carry no dir.
func presentApprovalRule(r approval.Rule) (protocol.ApprovalRule, error) {
	scope, ok := presentApprovalScope(r.Scope)
	if !ok {
		return protocol.ApprovalRule{}, fmt.Errorf("approval.listRules: unsupported scope %q", r.Scope)
	}
	decision, ok := presentApprovalDecision(r.Decision)
	if !ok {
		return protocol.ApprovalRule{}, fmt.Errorf("approval.listRules: unsupported decision %q", r.Decision)
	}
	wire := protocol.ApprovalRule{
		ID:       r.ID,
		Scope:    scope,
		Tool:     r.Tool,
		Subject:  r.Subject,
		Decision: decision,
	}
	if r.Scope == approval.ScopeProject {
		wire.Dir = r.ScopeKey
	}
	return wire, nil
}

func presentApprovalScope(scope approval.Scope) (protocol.ApprovalRuleScope, bool) {
	switch scope {
	case approval.ScopeSession:
		return protocol.ApprovalRuleScopeSession, true
	case approval.ScopeProject:
		return protocol.ApprovalRuleScopeProject, true
	case approval.ScopeGlobal:
		return protocol.ApprovalRuleScopeGlobal, true
	default:
		return "", false
	}
}

func presentApprovalDecision(decision approval.Decision) (protocol.ApprovalRuleDecision, bool) {
	switch decision {
	case approval.Allow:
		return protocol.ApprovalRuleDecisionAllow, true
	case approval.Deny:
		return protocol.ApprovalRuleDecisionDeny, true
	default:
		return "", false
	}
}

// presentApprovalMode maps the complete domain vocabulary to protocol names. An
// unknown value is rejected rather than disguised as a valid stance.
func presentApprovalMode(m approval.Mode) (protocol.ApprovalMode, bool) {
	switch m {
	case approval.ModeSafe:
		return protocol.ApprovalModeSafe, true
	case approval.ModeBalanced:
		return protocol.ApprovalModeBalanced, true
	case approval.ModeYolo:
		return protocol.ApprovalModeYolo, true
	default:
		return "", false
	}
}

// rememberScopeFromWire validates a wire remember scope; ok=false for an
// unknown value (the caller raises invalid_params, like approvalModeFromWire).
// The returned domain scope keeps the protocol vocabulary from leaking into
// application commands while the boundary still rejects unknown wire values.
func rememberScopeFromWire(s protocol.RememberScopeKind) (approval.Scope, bool) {
	switch s {
	case protocol.RememberSession:
		return approval.ScopeSession, true
	case protocol.RememberProject:
		return approval.ScopeProject, true
	case protocol.RememberGlobal:
		return approval.ScopeGlobal, true
	}
	return "", false
}

// approvalModeFromWire maps a protocol stance to the domain stance; ok=false for an
// unknown value (the caller raises invalid_params).
func approvalModeFromWire(m protocol.ApprovalMode) (approval.Mode, bool) {
	switch m {
	case protocol.ApprovalModeSafe:
		return approval.ModeSafe, true
	case protocol.ApprovalModeBalanced:
		return approval.ModeBalanced, true
	case protocol.ApprovalModeYolo:
		return approval.ModeYolo, true
	}
	return "", false
}
