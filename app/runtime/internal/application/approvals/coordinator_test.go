package approvals

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
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

func (a *approvalStore) DefaultMode(context.Context) (approval.Mode, error) { return a.mode, nil }

func (a *approvalStore) SetDefaultMode(_ context.Context, mode approval.Mode) error {
	a.set = append(a.set, mode)
	return nil
}

func (a *approvalStore) Rules(_ context.Context, sessionID, projectDir string) ([]approval.Rule, error) {
	a.ruleScopes = append(a.ruleScopes, approvalRuleScope{sessionID: sessionID, projectDir: projectDir})
	return a.rules, nil
}

func (a *approvalStore) Forget(_ context.Context, id string) error {
	a.forgotten = append(a.forgotten, id)
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

func TestDefaultModeUsesModePorts(t *testing.T) {
	store := &approvalStore{mode: approval.ModeBalanced}
	c := New(store, nil)

	got, err := c.DefaultMode(context.Background())
	if err != nil {
		t.Fatalf("DefaultMode: %v", err)
	}
	if got != approval.ModeBalanced {
		t.Fatalf("mode = %v, want balanced", got)
	}

	if err := c.SetDefaultMode(context.Background(), approval.ModeYolo); err != nil {
		t.Fatalf("SetDefaultMode: %v", err)
	}
	if len(store.set) != 1 || store.set[0] != approval.ModeYolo {
		t.Fatalf("set calls = %+v, want yolo", store.set)
	}
}

func TestListRulesResolvesSessionProject(t *testing.T) {
	store := &approvalStore{}
	c := New(store, fakeSessionLookup{sess: sessionfixture.MustRestore(session.Snapshot{
		ID: "ses_1", Workspace: sessionfixture.MustWorkspace("/repo"),
	})})

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
