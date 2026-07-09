package approval_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval/approvaltest"
)

// Black-box round-trips through the exported Policy API against the in-memory
// store fixture. The white-box matching/precedence tests (unexported subject /
// matchesSubject / ruleSet.decide) stay in rule_test.go.

// TestServiceRememberDecide: a remembered shell command auto-resolves a matching
// future call; a different command still misses (subject granularity).
func TestServiceRememberDecide(t *testing.T) {
	ctx := context.Background()
	svc := approval.New(approval.ModeSafe, approvaltest.NewMemoryStore())
	build := `{"command":"npm run build"}`
	_ = svc.Remember(ctx, approval.RememberRequest{
		Scope: approval.ScopeSession, SessionID: "s1", Tool: "shell", Arguments: build, Decision: approval.Allow,
	})

	if d, ok, _ := svc.Decide(ctx, approval.Query{SessionID: "s1", Tool: "shell", Arguments: build}); !ok || d != approval.Allow {
		t.Fatalf("matching call = (%v,%v), want (allow,true)", d, ok)
	}
	// A different command isn't covered by the remembered one.
	if _, ok, _ := svc.Decide(ctx, approval.Query{SessionID: "s1", Tool: "shell", Arguments: `{"command":"rm -rf /"}`}); ok {
		t.Fatal("a remembered `npm run build` rule matched `rm -rf /`")
	}
}

// TestServiceScopeVisibilityAndForget: a project rule is invisible from another
// dir; Forget(id) removes it.
func TestServiceScopeVisibilityAndForget(t *testing.T) {
	ctx := context.Background()
	svc := approval.New(approval.ModeSafe, approvaltest.NewMemoryStore())
	_ = svc.Remember(ctx, approval.RememberRequest{
		Scope: approval.ScopeProject, ProjectDir: "/proj/a", Tool: "write", Arguments: `{"file_path":"x"}`, Decision: approval.Allow,
	})

	q := approval.Query{SessionID: "s1", ProjectDir: "/proj/a", Tool: "write", Arguments: `{"file_path":"x"}`}
	if _, ok, _ := svc.Decide(ctx, q); !ok {
		t.Fatal("project rule not visible from its own dir")
	}
	other := q
	other.ProjectDir = "/proj/b"
	if _, ok, _ := svc.Decide(ctx, other); ok {
		t.Fatal("project rule leaked to another dir")
	}

	rules, _ := svc.Rules(ctx, "s1", "/proj/a")
	if len(rules) != 1 {
		t.Fatalf("Rules = %d, want 1", len(rules))
	}
	if err := svc.Forget(ctx, rules[0].ID); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if _, ok, _ := svc.Decide(ctx, q); ok {
		t.Fatal("rule still matched after Forget")
	}
}

// TestRememberDropsUnkeyable: a project rule with no cwd can't be keyed, so it
// is dropped rather than stored under an empty key (which would leak).
func TestRememberDropsUnkeyable(t *testing.T) {
	ctx := context.Background()
	svc := approval.New(approval.ModeSafe, approvaltest.NewMemoryStore())
	_ = svc.Remember(ctx, approval.RememberRequest{
		Scope: approval.ScopeProject, ProjectDir: "", Tool: "shell", Arguments: `{}`, Decision: approval.Allow,
	})
	if rules, _ := svc.Rules(ctx, "s1", ""); len(rules) != 0 {
		t.Fatalf("unkeyable project rule stored: %+v", rules)
	}
}
