package capabilities

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

type approvalStore struct {
	mode       approval.Mode
	set        []approval.Mode
	rules      []approval.Rule
	ruleScopes []approvalRuleScope
	forgotten  []string
}

type approvalRuleScope struct {
	sessionID  string
	projectDir string
}

func (s *approvalStore) Mode(context.Context) (approval.Mode, error) { return s.mode, nil }

func (s *approvalStore) SetMode(_ context.Context, mode approval.Mode) error {
	s.set = append(s.set, mode)
	return nil
}

func (s *approvalStore) Rules(_ context.Context, sessionID, projectDir string) ([]approval.Rule, error) {
	s.ruleScopes = append(s.ruleScopes, approvalRuleScope{sessionID: sessionID, projectDir: projectDir})
	return s.rules, nil
}

func (s *approvalStore) Forget(_ context.Context, id string) error {
	s.forgotten = append(s.forgotten, id)
	return nil
}

func (*approvalStore) Decide(context.Context, approval.Query) (approval.Decision, bool, error) {
	return "", false, nil
}

func (*approvalStore) Remember(context.Context, approval.RememberRequest) error { return nil }

// fakeSessionLookup stubs the session getter the approval-rule scoping reads.
type fakeSessionLookup struct {
	sess session.Session
	err  error
}

func (f fakeSessionLookup) Get(context.Context, string) (session.Session, error) {
	return f.sess, f.err
}

func TestApprovalModeUsesModePorts(t *testing.T) {
	approvals := &approvalStore{mode: approval.ModeBalanced}
	c := New(Config{Approval: approvals})

	got, err := c.ApprovalMode(context.Background())
	if err != nil {
		t.Fatalf("ApprovalMode: %v", err)
	}
	if got != approval.ModeBalanced {
		t.Fatalf("mode = %v, want balanced", got)
	}

	if err := c.SetApprovalMode(context.Background(), approval.ModeYolo); err != nil {
		t.Fatalf("SetApprovalMode: %v", err)
	}
	if len(approvals.set) != 1 || approvals.set[0] != approval.ModeYolo {
		t.Fatalf("set calls = %+v, want yolo", approvals.set)
	}
}

func TestListApprovalRulesResolvesSessionProject(t *testing.T) {
	approvals := &approvalStore{}
	c := New(Config{Approval: approvals, Sessions: fakeSessionLookup{sess: session.Session{ID: "ses_1", Cwd: "/repo"}}})

	if _, err := c.ListApprovalRules(context.Background(), "ses_1"); err != nil {
		t.Fatalf("list approval rules: %v", err)
	}
	if len(approvals.ruleScopes) != 1 {
		t.Fatalf("rule calls = %d, want 1", len(approvals.ruleScopes))
	}
	if got := approvals.ruleScopes[0]; got.sessionID != "ses_1" || got.projectDir != "/repo" {
		t.Fatalf("rule scope = %+v, want session ses_1 project /repo", got)
	}
}

func TestListApprovalRulesUnknownSessionUsesEmptyProject(t *testing.T) {
	approvals := &approvalStore{}
	c := New(Config{Approval: approvals, Sessions: fakeSessionLookup{err: session.ErrNotFound}})

	if _, err := c.ListApprovalRules(context.Background(), "missing"); err != nil {
		t.Fatalf("list approval rules: %v", err)
	}
	if got := approvals.ruleScopes[0]; got.sessionID != "missing" || got.projectDir != "" {
		t.Fatalf("rule scope = %+v, want missing session with empty project", got)
	}
}

func TestListApprovalRulesReturnsSessionStoreFailure(t *testing.T) {
	storeErr := errors.New("store unavailable")
	approvals := &approvalStore{}
	c := New(Config{Approval: approvals, Sessions: fakeSessionLookup{err: storeErr}})

	_, err := c.ListApprovalRules(context.Background(), "ses_1")
	if !errors.Is(err, storeErr) {
		t.Fatalf("list approval rules err = %v, want %v", err, storeErr)
	}
	if len(approvals.ruleScopes) != 0 {
		t.Fatalf("approval rules called after session failure: %+v", approvals.ruleScopes)
	}
}

func TestForgetApprovalRuleUsesDeletionPort(t *testing.T) {
	approvals := &approvalStore{}
	c := New(Config{Approval: approvals})

	if err := c.ForgetApprovalRule(context.Background(), "rule_1"); err != nil {
		t.Fatalf("ForgetApprovalRule: %v", err)
	}
	if len(approvals.forgotten) != 1 || approvals.forgotten[0] != "rule_1" {
		t.Fatalf("forgotten = %+v, want rule_1", approvals.forgotten)
	}
}
