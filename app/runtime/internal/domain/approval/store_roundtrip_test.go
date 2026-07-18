package approval_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval/approvaltest"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// Black-box round-trips through the exported Policy API against the in-memory
// store fixture. The white-box matching/precedence tests (unexported subject /
// matchesSubject / ruleSet.decide) stay in rule_test.go.

// TestServiceRememberDecide: a remembered shell command auto-resolves a matching
// future call; a different command still misses (subject granularity).
func TestServiceRememberDecide(t *testing.T) {
	ctx := context.Background()
	svc := newPolicy(t)
	build := parseArguments(t, `{"command":"npm run build"}`)
	if err := svc.Remember(ctx, approval.RememberRequest{
		Scope: approval.ScopeSession, SessionID: "s1", Tool: "shell", Arguments: build, Decision: approval.Allow,
	}); err != nil {
		t.Fatalf("remember: %v", err)
	}

	if d, ok, err := svc.Decide(ctx, approval.Query{SessionID: "s1", Tool: "shell", Arguments: build}); err != nil || !ok || d != approval.Allow {
		t.Fatalf("matching call = (%v,%v,%v), want (allow,true,nil)", d, ok, err)
	}
	// A different command isn't covered by the remembered one.
	if _, ok, err := svc.Decide(ctx, approval.Query{SessionID: "s1", Tool: "shell", Arguments: parseArguments(t, `{"command":"rm -rf /"}`)}); err != nil || ok {
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
	arguments := parseArguments(t, `{"file_path":"x"}`)
	if err := svc.Remember(ctx, approval.RememberRequest{
		Scope: approval.ScopeProject, ProjectDir: "/proj/a", Tool: "write", Arguments: arguments, Decision: approval.Allow,
	}); err != nil {
		t.Fatalf("remember: %v", err)
	}

	q := approval.Query{SessionID: "s1", ProjectDir: "/proj/a", Tool: "write", Arguments: arguments}
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
		Arguments: parseArguments(t, `{"command":"go test"}`), Decision: approval.Allow,
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

func newPolicy(t *testing.T) approval.Policy {
	t.Helper()
	policy, err := approval.New(approval.ModeSafe, approvaltest.NewMemoryStore())
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}
	return policy
}

func parseArguments(t *testing.T, raw string) tool.Arguments {
	t.Helper()
	arguments, err := tool.ParseArguments(raw)
	if err != nil {
		t.Fatalf("parse arguments: %v", err)
	}
	return arguments
}
