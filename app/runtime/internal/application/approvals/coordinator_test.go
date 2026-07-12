package approvals

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

func TestModeUsesModePorts(t *testing.T) {
	store := &approvalStore{mode: approval.ModeBalanced}
	c := New(store, nil)

	got, err := c.Mode(context.Background())
	if err != nil {
		t.Fatalf("Mode: %v", err)
	}
	if got != approval.ModeBalanced {
		t.Fatalf("mode = %v, want balanced", got)
	}

	if err := c.SetMode(context.Background(), approval.ModeYolo); err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	if len(store.set) != 1 || store.set[0] != approval.ModeYolo {
		t.Fatalf("set calls = %+v, want yolo", store.set)
	}
}

func TestListRulesResolvesSessionProject(t *testing.T) {
	store := &approvalStore{}
	c := New(store, fakeSessionLookup{sess: session.Session{ID: "ses_1", Cwd: "/repo"}})

	if _, err := c.ListRules(context.Background(), "ses_1"); err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(store.ruleScopes) != 1 {
		t.Fatalf("rule calls = %d, want 1", len(store.ruleScopes))
	}
	if got := store.ruleScopes[0]; got.sessionID != "ses_1" || got.projectDir != "/repo" {
		t.Fatalf("rule scope = %+v, want session ses_1 project /repo", got)
	}
}

func TestListRulesUnknownSessionUsesEmptyProject(t *testing.T) {
	store := &approvalStore{}
	c := New(store, fakeSessionLookup{err: session.ErrNotFound})

	if _, err := c.ListRules(context.Background(), "missing"); err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if got := store.ruleScopes[0]; got.sessionID != "missing" || got.projectDir != "" {
		t.Fatalf("rule scope = %+v, want missing session with empty project", got)
	}
}

func TestListRulesReturnsSessionStoreFailure(t *testing.T) {
	storeErr := errors.New("store unavailable")
	store := &approvalStore{}
	c := New(store, fakeSessionLookup{err: storeErr})

	_, err := c.ListRules(context.Background(), "ses_1")
	if !errors.Is(err, storeErr) {
		t.Fatalf("list rules err = %v, want %v", err, storeErr)
	}
	if len(store.ruleScopes) != 0 {
		t.Fatalf("rules called after session failure: %+v", store.ruleScopes)
	}
}

func TestForgetRuleUsesDeletionPort(t *testing.T) {
	store := &approvalStore{}
	c := New(store, nil)

	if err := c.ForgetRule(context.Background(), "rule_1"); err != nil {
		t.Fatalf("ForgetRule: %v", err)
	}
	if len(store.forgotten) != 1 || store.forgotten[0] != "rule_1" {
		t.Fatalf("forgotten = %+v, want rule_1", store.forgotten)
	}
}
