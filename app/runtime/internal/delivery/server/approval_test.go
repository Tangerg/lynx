package server

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
)

type approvalRuntime struct {
	stubRuntime
	mode             approval.Mode
	set              []approval.Mode
	rules            []approval.Rule
	rulesForSession  string
	forgottenRuleIDs []string
}

func (r *approvalRuntime) GetApprovalMode(context.Context) (approval.Mode, error) {
	return r.mode, nil
}

func (r *approvalRuntime) SetApprovalMode(_ context.Context, mode approval.Mode) error {
	r.set = append(r.set, mode)
	return nil
}

func (r *approvalRuntime) ListApprovalRules(_ context.Context, sessionID string) ([]approval.Rule, error) {
	r.rulesForSession = sessionID
	return r.rules, nil
}

func (r *approvalRuntime) ForgetApprovalRule(_ context.Context, id string) error {
	r.forgottenRuleIDs = append(r.forgottenRuleIDs, id)
	return nil
}

// TestApprovalModeWireRoundTrip checks every engine stance maps to a wire name
// and back, and that an unknown wire value is rejected (→ invalid_params).
func TestApprovalModeWireRoundTrip(t *testing.T) {
	for _, m := range []approval.Mode{approval.ModeSafe, approval.ModeBalanced, approval.ModeYolo, approval.ModePlan} {
		back, ok := approvalModeFromWire(approvalModeToWire(m))
		if !ok || back != m {
			t.Errorf("round-trip %v → %q → %v (ok=%v)", m, approvalModeToWire(m), back, ok)
		}
	}
	if _, ok := approvalModeFromWire(protocol.ApprovalMode("bogus")); ok {
		t.Error("unknown wire approval mode must be rejected")
	}
}

func TestApprovalModeHandlersUseRuntimeFacade(t *testing.T) {
	rt := &approvalRuntime{mode: approval.ModePlan}
	s := newTestServer(rt)

	got, err := s.GetApprovalMode(context.Background())
	if err != nil {
		t.Fatalf("get approval mode: %v", err)
	}
	if got.Mode != protocol.ApprovalModePlan {
		t.Fatalf("mode = %q, want plan", got.Mode)
	}

	got, err = s.SetApprovalMode(context.Background(), protocol.SetApprovalModeRequest{Mode: protocol.ApprovalModeBalanced})
	if err != nil {
		t.Fatalf("set approval mode: %v", err)
	}
	if got.Mode != protocol.ApprovalModeBalanced {
		t.Fatalf("mode = %q, want balanced", got.Mode)
	}
	if len(rt.set) != 1 || rt.set[0] != approval.ModeBalanced {
		t.Fatalf("set modes = %+v, want balanced", rt.set)
	}
}

func TestListApprovalRulesUsesRuntimeFacade(t *testing.T) {
	rt := &approvalRuntime{rules: []approval.Rule{{
		ID:       "rule_1",
		Scope:    approval.ScopeProject,
		ScopeKey: "/repo",
		Tool:     "shell",
		Subject:  "npm test",
		Decision: approval.Allow,
	}}}
	s := newTestServer(rt)

	got, err := s.ListApprovalRules(context.Background(), protocol.ListApprovalRulesRequest{SessionID: "ses_1"})
	if err != nil {
		t.Fatalf("list approval rules: %v", err)
	}
	if rt.rulesForSession != "ses_1" {
		t.Fatalf("runtime session = %q, want ses_1", rt.rulesForSession)
	}
	if len(got.Rules) != 1 || got.Rules[0].Dir != "/repo" || got.Rules[0].Decision != string(approval.Allow) {
		t.Fatalf("wire rules = %+v", got.Rules)
	}
}

func TestForgetApprovalRuleUsesRuntimeFacade(t *testing.T) {
	rt := &approvalRuntime{}
	s := newTestServer(rt)

	if err := s.ForgetApprovalRule(context.Background(), protocol.ForgetApprovalRuleRequest{ID: "rule_1"}); err != nil {
		t.Fatalf("forget approval rule: %v", err)
	}
	if len(rt.forgottenRuleIDs) != 1 || rt.forgottenRuleIDs[0] != "rule_1" {
		t.Fatalf("forgotten = %+v, want rule_1", rt.forgottenRuleIDs)
	}
}
