package approval

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// White-box tests: the matching/precedence rules are the heart of the rule
// engine, so they're exercised directly alongside the policy round-trips.

func TestModeGetSet(t *testing.T) {
	svc := mustPolicy(t, ModeYolo, nil)
	if m, _ := svc.Mode(context.Background()); m != ModeYolo {
		t.Fatalf("initial mode = %v, want Yolo", m)
	}
	if err := svc.SetMode(context.Background(), ModeBalanced); err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	if m, _ := svc.Mode(context.Background()); m != ModeBalanced {
		t.Fatalf("mode after set = %v, want Balanced", m)
	}
}

func TestQuerySubject(t *testing.T) {
	cases := []struct {
		tool, arguments, want string
		wantError             bool
	}{
		{tool: "shell", arguments: `{"command":"npm run build"}`, want: "npm run build"},
		{tool: "run_in_background", arguments: `{"command":"sleep 1"}`, want: "sleep 1"},
		{tool: "edit", arguments: `{"file_path":"src/a.go","old":"x"}`, want: "src/a.go"},
		{tool: "read", arguments: `{"file_path":"go.mod"}`, want: "go.mod"},
		{tool: "grep", arguments: `{"pattern":"foo"}`}, // no per-tool subject → whole-tool
		{tool: "shell", arguments: `{"timeout":5}`, wantError: true},
	}
	for _, c := range cases {
		q := Query{Tool: c.tool, Arguments: mustArguments(t, c.arguments)}
		got, err := q.subject()
		if c.wantError {
			if !errors.Is(err, ErrInvalidQuery) || !errors.Is(err, tool.ErrInvalidArguments) {
				t.Errorf("Query.subject(%q,%q) error = %v", c.tool, c.arguments, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("Query.subject(%q,%q) = (%q, %v), want %q", c.tool, c.arguments, got, err, c.want)
		}
	}
}

func TestRuleMatchesSubject(t *testing.T) {
	cases := []struct {
		pattern, subject string
		want             bool
	}{
		{"", "anything", true},                   // any
		{"npm run build", "npm run build", true}, // exact hit
		{"npm run build", "npm test", false},     // exact miss
		{"npm run *", "npm run build", true},     // glob hit
		{"npm run *", "yarn build", false},       // glob miss
		{"src/*.go", "src/a.go", true},           // path glob, one level
		{"src/*.go", "src/sub/a.go", false},      // * does not cross /
	}
	for _, c := range cases {
		if got := (Rule{Subject: c.pattern}).matchesSubject(c.subject); got != c.want {
			t.Errorf("Rule.matchesSubject(%q,%q) = %v, want %v", c.pattern, c.subject, got, c.want)
		}
	}
}

// TestDecidePrecedence: the most specific matching rule wins — scope dominates
// (session > project > global), then subject (exact > glob > any).
func TestDecidePrecedence(t *testing.T) {
	q := Query{SessionID: "s1", ProjectDir: "/p", Tool: "shell", Arguments: mustArguments(t, `{"command":"rm -rf /"}`)}

	// A broad session allow vs a narrow (exact-subject) session deny → deny wins.
	rules := []Rule{
		mustRule(t, ScopeSession, "s1", "shell", "", Allow),
		mustRule(t, ScopeSession, "s1", "shell", "rm -rf /", Deny),
	}
	if d, ok, err := ruleSet(rules).decide(q); err != nil || !ok || d != Deny {
		t.Fatalf("exact deny over broad allow = (%v,%v,%v), want (deny,true,nil)", d, ok, err)
	}

	// A global deny vs a session allow (both whole-tool) → session allow wins.
	rules = []Rule{
		mustRule(t, ScopeGlobal, "", "shell", "", Deny),
		mustRule(t, ScopeSession, "s1", "shell", "", Allow),
	}
	if d, ok, err := ruleSet(rules).decide(q); err != nil || !ok || d != Allow {
		t.Fatalf("session allow over global deny = (%v,%v,%v), want (allow,true,nil)", d, ok, err)
	}

	// Wrong tool / no rules → miss.
	if _, ok, err := (ruleSet{mustRule(t, ScopeSession, "s1", "write", "", Allow)}).decide(q); err != nil || ok {
		t.Fatal("a write rule matched a shell call")
	}
}

// TestDecideConflictDeny: two equally-specific rules disagree → deny wins (a
// remembered deny must not be overridden by an equally-specific allow).
func TestDecideConflictDeny(t *testing.T) {
	q := Query{SessionID: "s1", Tool: "shell", Arguments: mustArguments(t, `{"command":"go test"}`)}
	rules := []Rule{
		mustRule(t, ScopeSession, "s1", "shell", "", Allow),
		mustRule(t, ScopeSession, "s1", "shell", "", Deny),
	}
	if d, ok, err := ruleSet(rules).decide(q); err != nil || !ok || d != Deny {
		t.Fatalf("conflict = (%v,%v,%v), want (deny,true,nil)", d, ok, err)
	}
}

// TestNilStore: a service with no store remembers nothing and matches nothing.
func TestNilStore(t *testing.T) {
	ctx := context.Background()
	svc := mustPolicy(t, ModeSafe, nil)
	arguments := mustArguments(t, `{"command":"go test"}`)
	if err := svc.Remember(ctx, RememberRequest{Scope: ScopeGlobal, Tool: "shell", Arguments: arguments, Decision: Allow}); !errors.Is(err, ErrRuleStoreUnavailable) {
		t.Fatalf("Remember on nil store error = %v, want ErrRuleStoreUnavailable", err)
	}
	if _, ok, _ := svc.Decide(ctx, Query{Tool: "shell", Arguments: arguments}); ok {
		t.Fatal("nil store matched a rule")
	}
	if rules, _ := svc.Rules(ctx, "s1", "/p"); rules != nil {
		t.Fatalf("nil store Rules = %+v, want nil", rules)
	}
	if err := svc.Forget(ctx, "rule_missing"); !errors.Is(err, ErrRuleStoreUnavailable) {
		t.Fatalf("Forget on nil store error = %v, want ErrRuleStoreUnavailable", err)
	}
}

func TestPolicyRejectsInvalidMode(t *testing.T) {
	if _, err := New(Mode(255), nil); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("New invalid mode error = %v, want ErrInvalidMode", err)
	}
	svc := mustPolicy(t, ModeSafe, nil)
	if err := svc.SetMode(t.Context(), Mode(255)); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("SetMode invalid mode error = %v, want ErrInvalidMode", err)
	}
	if got, err := svc.Mode(t.Context()); err != nil || got != ModeSafe {
		t.Fatalf("mode after rejected update = (%v, %v), want safe", got, err)
	}
}

func TestRuleValidationRejectsCorruptDurableValues(t *testing.T) {
	valid := mustRule(t, ScopeProject, "/repo", "shell", "npm run *", Allow)
	tests := []struct {
		name   string
		mutate func(*Rule)
	}{
		{name: "identity drift", mutate: func(rule *Rule) { rule.Tool = "write" }},
		{name: "unknown scope", mutate: func(rule *Rule) { rule.Scope = Scope("team") }},
		{name: "missing scope key", mutate: func(rule *Rule) { rule.ScopeKey = "" }},
		{name: "unknown decision", mutate: func(rule *Rule) { rule.Decision = Decision("maybe") }},
		{name: "invalid glob", mutate: func(rule *Rule) { rule.Subject = "[" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rule := valid
			test.mutate(&rule)
			if err := rule.Validate(); !errors.Is(err, ErrInvalidRule) {
				t.Fatalf("Validate error = %v, want ErrInvalidRule", err)
			}
		})
	}
}

func TestPolicyRejectsMissingRequiredRuleSubject(t *testing.T) {
	svc := mustPolicy(t, ModeSafe, nil)
	missingCommand := mustArguments(t, `{"timeout":30}`)
	if _, _, err := svc.Decide(t.Context(), Query{Tool: "shell", Arguments: missingCommand}); !errors.Is(err, ErrInvalidQuery) || !errors.Is(err, tool.ErrInvalidArguments) {
		t.Fatalf("Decide error = %v, want invalid query + invalid arguments", err)
	}
	if err := svc.Remember(t.Context(), RememberRequest{
		Scope: ScopeGlobal, Tool: "shell", Arguments: missingCommand, Decision: Allow,
	}); !errors.Is(err, ErrInvalidRule) || !errors.Is(err, tool.ErrInvalidArguments) {
		t.Fatalf("Remember error = %v, want invalid rule + invalid arguments", err)
	}
}

func mustPolicy(t *testing.T, mode Mode, store RuleStore) *RuntimePolicy {
	t.Helper()
	policy, err := New(mode, store)
	if err != nil {
		t.Fatalf("New policy: %v", err)
	}
	return policy
}

func mustArguments(t *testing.T, raw string) tool.Arguments {
	t.Helper()
	arguments, err := tool.ParseArguments(raw)
	if err != nil {
		t.Fatalf("ParseArguments(%q): %v", raw, err)
	}
	return arguments
}

func mustRule(t *testing.T, scope Scope, scopeKey, toolName, subject string, decision Decision) Rule {
	t.Helper()
	rule, err := NewRule(scope, scopeKey, toolName, subject, decision)
	if err != nil {
		t.Fatalf("NewRule: %v", err)
	}
	return rule
}
