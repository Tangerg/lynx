package approval_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
)

// White-box tests: the matching/precedence rules are the heart of the rule
// engine, so they're exercised directly alongside the policy round-trips.

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
		rule := mustRule(t, approval.ScopeGlobal, "", "shell", c.pattern, approval.Allow)
		_, got, err := approval.Decide([]approval.Rule{rule}, approval.Query{Tool: "shell", Subject: c.subject})
		if err != nil || got != c.want {
			t.Errorf("Decide subject(%q,%q) = %v, %v; want match %v", c.pattern, c.subject, got, err, c.want)
		}
	}
}

// TestDecidePrecedence: the most specific matching rule wins — scope dominates
// (session > project > global), then subject (exact > glob > any).
func TestDecidePrecedence(t *testing.T) {
	q := approval.Query{SessionID: "s1", ProjectDir: "/p", Tool: "shell", Subject: "rm -rf /"}

	// A broad session allow vs a narrow (exact-subject) session deny → deny wins.
	rules := []approval.Rule{
		mustRule(t, approval.ScopeSession, "s1", "shell", "", approval.Allow),
		mustRule(t, approval.ScopeSession, "s1", "shell", "rm -rf /", approval.Deny),
	}
	if d, ok, err := approval.Decide(rules, q); err != nil || !ok || d != approval.Deny {
		t.Fatalf("exact deny over broad allow = (%v,%v,%v), want (deny,true,nil)", d, ok, err)
	}

	// A global deny vs a session allow (both whole-tool) → session allow wins.
	rules = []approval.Rule{
		mustRule(t, approval.ScopeGlobal, "", "shell", "", approval.Deny),
		mustRule(t, approval.ScopeSession, "s1", "shell", "", approval.Allow),
	}
	if d, ok, err := approval.Decide(rules, q); err != nil || !ok || d != approval.Allow {
		t.Fatalf("session allow over global deny = (%v,%v,%v), want (allow,true,nil)", d, ok, err)
	}

	// Wrong tool / no rules → miss.
	if _, ok, err := approval.Decide([]approval.Rule{mustRule(t, approval.ScopeSession, "s1", "write", "", approval.Allow)}, q); err != nil || ok {
		t.Fatal("a write rule matched a shell call")
	}
}

// TestDecideConflictDeny: two equally-specific rules disagree → deny wins (a
// remembered deny must not be overridden by an equally-specific allow).
func TestDecideConflictDeny(t *testing.T) {
	q := approval.Query{SessionID: "s1", Tool: "shell", Subject: "go test"}
	rules := []approval.Rule{
		mustRule(t, approval.ScopeSession, "s1", "shell", "", approval.Allow),
		mustRule(t, approval.ScopeSession, "s1", "shell", "", approval.Deny),
	}
	if d, ok, err := approval.Decide(rules, q); err != nil || !ok || d != approval.Deny {
		t.Fatalf("conflict = (%v,%v,%v), want (deny,true,nil)", d, ok, err)
	}
}

func TestSessionModeValidation(t *testing.T) {
	tests := []struct {
		name  string
		state approval.SessionMode
		valid bool
	}{
		{name: "Plan restores safe", state: approval.SessionMode{Mode: approval.ModePlan, RestoreMode: approval.ModeSafe}, valid: true},
		{name: "Plan restores balanced", state: approval.SessionMode{Mode: approval.ModePlan, RestoreMode: approval.ModeBalanced}, valid: true},
		{name: "explicit yolo", state: approval.SessionMode{Mode: approval.ModeYolo}, valid: true},
		{name: "Plan cannot restore Plan", state: approval.SessionMode{Mode: approval.ModePlan, RestoreMode: approval.ModePlan}},
		{name: "unknown mode", state: approval.SessionMode{Mode: approval.Mode("invalid")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.state.Validate()
			if test.valid && err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if !test.valid && !errors.Is(err, approval.ErrInvalidSessionMode) {
				t.Fatalf("Validate error = %v, want ErrInvalidSessionMode", err)
			}
		})
	}
}

func TestRuleValidationRejectsCorruptDurableValues(t *testing.T) {
	valid := mustRule(t, approval.ScopeProject, "/repo", "shell", "npm run *", approval.Allow)
	tests := []struct {
		name   string
		mutate func(*approval.Rule)
	}{
		{name: "identity drift", mutate: func(rule *approval.Rule) { rule.Tool = "write" }},
		{name: "unknown scope", mutate: func(rule *approval.Rule) { rule.Scope = approval.Scope("team") }},
		{name: "missing scope key", mutate: func(rule *approval.Rule) { rule.ScopeKey = "" }},
		{name: "unknown decision", mutate: func(rule *approval.Rule) { rule.Decision = approval.Decision("maybe") }},
		{name: "invalid glob", mutate: func(rule *approval.Rule) { rule.Subject = "[" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rule := valid
			test.mutate(&rule)
			if err := rule.Validate(); !errors.Is(err, approval.ErrInvalidRule) {
				t.Fatalf("Validate error = %v, want ErrInvalidRule", err)
			}
		})
	}
}

func mustRule(t *testing.T, scope approval.Scope, scopeKey, toolName, subject string, decision approval.Decision) approval.Rule {
	t.Helper()
	rule, err := approval.NewRule(scope, scopeKey, toolName, subject, decision)
	if err != nil {
		t.Fatalf("NewRule: %v", err)
	}
	return rule
}
