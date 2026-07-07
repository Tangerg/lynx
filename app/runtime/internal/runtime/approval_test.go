package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

type approvalSessionStore struct {
	session.Store
	sess session.Session
	err  error
}

func (s approvalSessionStore) Get(context.Context, string) (session.Session, error) {
	return s.sess, s.err
}

type approvalStore struct {
	approval.Policy
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

func (s *approvalStore) Mode(context.Context) (approval.Mode, error) {
	return s.mode, nil
}

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

func TestRuntimeListApprovalRulesResolvesSessionProject(t *testing.T) {
	approvals := &approvalStore{}
	rt := &Runtime{
		session:  approvalSessionStore{sess: session.Session{ID: "ses_1", Cwd: "/repo"}},
		approval: approvals,
	}

	if _, err := rt.ListApprovalRules(context.Background(), "ses_1"); err != nil {
		t.Fatalf("list approval rules: %v", err)
	}
	if len(approvals.ruleScopes) != 1 {
		t.Fatalf("rule calls = %d, want 1", len(approvals.ruleScopes))
	}
	got := approvals.ruleScopes[0]
	if got.sessionID != "ses_1" || got.projectDir != "/repo" {
		t.Fatalf("rule scope = %+v, want session ses_1 project /repo", got)
	}
}

func TestRuntimeListApprovalRulesUnknownSessionUsesEmptyProject(t *testing.T) {
	approvals := &approvalStore{}
	rt := &Runtime{
		session:  approvalSessionStore{err: session.ErrNotFound},
		approval: approvals,
	}

	if _, err := rt.ListApprovalRules(context.Background(), "missing"); err != nil {
		t.Fatalf("list approval rules: %v", err)
	}
	if got := approvals.ruleScopes[0]; got.sessionID != "missing" || got.projectDir != "" {
		t.Fatalf("rule scope = %+v, want missing session with empty project", got)
	}
}

func TestRuntimeListApprovalRulesReturnsSessionStoreFailure(t *testing.T) {
	storeErr := errors.New("store unavailable")
	approvals := &approvalStore{}
	rt := &Runtime{
		session:  approvalSessionStore{err: storeErr},
		approval: approvals,
	}

	_, err := rt.ListApprovalRules(context.Background(), "ses_1")
	if !errors.Is(err, storeErr) {
		t.Fatalf("list approval rules err = %v, want %v", err, storeErr)
	}
	if len(approvals.ruleScopes) != 0 {
		t.Fatalf("approval rules called after session failure: %+v", approvals.ruleScopes)
	}
}
