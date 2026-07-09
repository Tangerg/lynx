package approval

import (
	"context"
	"testing"
)

// White-box tests: the matching/precedence rules are the heart of the rule
// engine, so they're exercised directly alongside the policy round-trips.

func TestModeGetSet(t *testing.T) {
	svc := New(ModeYolo, nil)
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
	cases := []struct{ tool, args, want string }{
		{"shell", `{"command":"npm run build"}`, "npm run build"},
		{"run_in_background", `{"command":"sleep 1"}`, "sleep 1"},
		{"edit", `{"file_path":"src/a.go","old":"x"}`, "src/a.go"},
		{"read", `{"file_path":"go.mod"}`, "go.mod"},
		{"grep", `{"pattern":"foo"}`, ""}, // no per-tool subject → whole-tool
		{"shell", `not json`, ""},         // unparseable → whole-tool
		{"shell", `{"timeout":5}`, ""},    // missing field → empty subject
	}
	for _, c := range cases {
		q := Query{Tool: c.tool, Arguments: c.args}
		if got := q.subject(); got != c.want {
			t.Errorf("Query.subject(%q,%q) = %q, want %q", c.tool, c.args, got, c.want)
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
	q := Query{SessionID: "s1", ProjectDir: "/p", Tool: "shell", Arguments: `{"command":"rm -rf /"}`}

	// A broad session allow vs a narrow (exact-subject) session deny → deny wins.
	rules := []Rule{
		{Scope: ScopeSession, Tool: "shell", Subject: "", Decision: Allow},
		{Scope: ScopeSession, Tool: "shell", Subject: "rm -rf /", Decision: Deny},
	}
	if d, ok := ruleSet(rules).decide(q); !ok || d != Deny {
		t.Fatalf("exact deny over broad allow = (%v,%v), want (deny,true)", d, ok)
	}

	// A global deny vs a session allow (both whole-tool) → session allow wins.
	rules = []Rule{
		{Scope: ScopeGlobal, Tool: "shell", Subject: "", Decision: Deny},
		{Scope: ScopeSession, Tool: "shell", Subject: "", Decision: Allow},
	}
	if d, ok := ruleSet(rules).decide(q); !ok || d != Allow {
		t.Fatalf("session allow over global deny = (%v,%v), want (allow,true)", d, ok)
	}

	// Wrong tool / no rules → miss.
	if _, ok := (ruleSet{{Scope: ScopeSession, Tool: "write", Decision: Allow}}).decide(q); ok {
		t.Fatal("a write rule matched a shell call")
	}
}

// TestDecideConflictDeny: two equally-specific rules disagree → deny wins (a
// remembered deny must not be overridden by an equally-specific allow).
func TestDecideConflictDeny(t *testing.T) {
	q := Query{SessionID: "s1", Tool: "shell", Arguments: `{}`}
	rules := []Rule{
		{Scope: ScopeSession, Tool: "shell", Subject: "", Decision: Allow},
		{Scope: ScopeSession, Tool: "shell", Subject: "", Decision: Deny},
	}
	if d, ok := ruleSet(rules).decide(q); !ok || d != Deny {
		t.Fatalf("conflict = (%v,%v), want (deny,true)", d, ok)
	}
}

// TestNilStore: a service with no store remembers nothing and matches nothing.
func TestNilStore(t *testing.T) {
	ctx := context.Background()
	svc := New(ModeSafe, nil)
	if err := svc.Remember(ctx, RememberRequest{Scope: ScopeGlobal, Tool: "shell", Decision: Allow}); err != nil {
		t.Fatalf("Remember on nil store: %v", err)
	}
	if _, ok, _ := svc.Decide(ctx, Query{Tool: "shell"}); ok {
		t.Fatal("nil store matched a rule")
	}
	if rules, _ := svc.Rules(ctx, "s1", "/p"); rules != nil {
		t.Fatalf("nil store Rules = %+v, want nil", rules)
	}
}
