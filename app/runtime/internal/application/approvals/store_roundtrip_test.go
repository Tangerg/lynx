package approvals_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/approvals"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
)

// Black-box round-trips through the exported Policy API against the in-memory
// store fixture. The white-box matching and precedence rules stay with their
// Domain owner in domain/approval/rule_test.go.

// TestServiceRememberDecide: a remembered shell command auto-resolves a matching
// future call; a different command still misses (subject granularity).
func TestServiceRememberDecide(t *testing.T) {
	ctx := context.Background()
	svc := newPolicy(t)
	if err := svc.Remember(ctx, approval.RememberRequest{
		Scope: approval.ScopeSession, SessionID: "s1", Tool: "shell", Subject: "npm run build", Decision: approval.Allow,
	}); err != nil {
		t.Fatalf("remember: %v", err)
	}

	if d, ok, err := svc.Decide(ctx, approval.Query{SessionID: "s1", Tool: "shell", Subject: "npm run build"}); err != nil || !ok || d != approval.Allow {
		t.Fatalf("matching call = (%v,%v,%v), want (allow,true,nil)", d, ok, err)
	}
	// A different command isn't covered by the remembered one.
	if _, ok, err := svc.Decide(ctx, approval.Query{SessionID: "s1", Tool: "shell", Subject: "rm -rf /"}); err != nil || ok {
		if err != nil {
			t.Fatalf("decide different command: %v", err)
		}
		t.Fatal("a remembered `npm run build` rule matched `rm -rf /`")
	}
}

// TestServiceScopeVisibilityAndForget: a project rule is invisible from another
// dir; Forget(id) removes it.
func TestServiceScopeVisibilityAndForget(t *testing.T) {
	ctx := context.Background()
	svc := newPolicy(t)
	if err := svc.Remember(ctx, approval.RememberRequest{
		Scope: approval.ScopeProject, ProjectDir: "/proj/a", Tool: "write", Subject: "x", Decision: approval.Allow,
	}); err != nil {
		t.Fatalf("remember: %v", err)
	}

	q := approval.Query{SessionID: "s1", ProjectDir: "/proj/a", Tool: "write", Subject: "x"}
	if _, ok, err := svc.Decide(ctx, q); err != nil || !ok {
		if err != nil {
			t.Fatalf("decide project rule: %v", err)
		}
		t.Fatal("project rule not visible from its own dir")
	}
	other := q
	other.ProjectDir = "/proj/b"
	if _, ok, err := svc.Decide(ctx, other); err != nil || ok {
		if err != nil {
			t.Fatalf("decide other project: %v", err)
		}
		t.Fatal("project rule leaked to another dir")
	}

	rules, err := svc.Rules(ctx, "s1", "/proj/a")
	if err != nil {
		t.Fatalf("Rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("Rules = %d, want 1", len(rules))
	}
	if err := svc.Forget(ctx, rules[0].ID); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if _, ok, err := svc.Decide(ctx, q); err != nil || ok {
		if err != nil {
			t.Fatalf("decide after forget: %v", err)
		}
		t.Fatal("rule still matched after Forget")
	}
}

// TestRememberRejectsUnkeyable prevents a missing project identity from being
// reported as remembered or leaking into a wider scope.
func TestRememberRejectsUnkeyable(t *testing.T) {
	ctx := context.Background()
	svc := newPolicy(t)
	err := svc.Remember(ctx, approval.RememberRequest{
		Scope: approval.ScopeProject, ProjectDir: "", Tool: "shell",
		Subject: "go test", Decision: approval.Allow,
	})
	if !errors.Is(err, approval.ErrInvalidRule) {
		t.Fatalf("unkeyable rule error = %v, want ErrInvalidRule", err)
	}
	if rules, listErr := svc.Rules(ctx, "s1", ""); listErr != nil || len(rules) != 0 {
		if listErr != nil {
			t.Fatalf("Rules: %v", listErr)
		}
		t.Fatalf("unkeyable project rule stored: %+v", rules)
	}
}

func newPolicy(t *testing.T) *approvals.RuntimePolicy {
	t.Helper()
	policy, err := approvals.NewRuntimePolicy(approval.ModeSafe, newMemoryRuleStore(), nil)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}
	return policy
}

// memoryRuleStore is this black-box test's in-process [approvals.RuleStore]. It
// stays beside its only consumer rather than creating a production-shaped
// package for one test fixture.
type memoryRuleStore struct {
	mu    sync.Mutex
	rules map[string]approval.Rule
}

func newMemoryRuleStore() *memoryRuleStore {
	return &memoryRuleStore{rules: make(map[string]approval.Rule)}
}

var _ approvals.RuleStore = (*memoryRuleStore)(nil)

func (s *memoryRuleStore) Put(_ context.Context, rule approval.Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules[rule.ID] = rule
	return nil
}

func (s *memoryRuleStore) Visible(_ context.Context, sessionID, projectDir string) ([]approval.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var rules []approval.Rule
	for _, rule := range s.rules {
		if ruleVisibleFrom(rule, sessionID, projectDir) {
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

func (s *memoryRuleStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rules, id)
	return nil
}

func (s *memoryRuleStore) DeleteSession(_ context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, rule := range s.rules {
		if rule.Scope == approval.ScopeSession && rule.ScopeKey == sessionID {
			delete(s.rules, id)
		}
	}
	return nil
}

func ruleVisibleFrom(rule approval.Rule, sessionID, projectDir string) bool {
	switch rule.Scope {
	case approval.ScopeSession:
		return rule.ScopeKey == sessionID
	case approval.ScopeProject:
		return projectDir != "" && rule.ScopeKey == projectDir
	case approval.ScopeGlobal:
		return true
	default:
		return false
	}
}
