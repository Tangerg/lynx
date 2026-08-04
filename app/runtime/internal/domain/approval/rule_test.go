package approval

import (
	"context"
	"errors"
	"testing"
)

// White-box tests: the matching/precedence rules are the heart of the rule
// engine, so they're exercised directly alongside the policy round-trips.

func TestDefaultModeGetSet(t *testing.T) {
	svc := mustPolicy(t, ModeYolo, nil)
	if m, _ := svc.DefaultMode(context.Background()); m != ModeYolo {
		t.Fatalf("initial mode = %v, want Yolo", m)
	}
	if err := svc.SetDefaultMode(context.Background(), ModeBalanced); err != nil {
		t.Fatalf("SetDefaultMode: %v", err)
	}
	if m, _ := svc.Mode(context.Background(), "session-1"); m != ModeBalanced {
		t.Fatalf("mode after set = %v, want Balanced", m)
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
	q := Query{SessionID: "s1", ProjectDir: "/p", Tool: "shell", Subject: "rm -rf /"}

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
	q := Query{SessionID: "s1", Tool: "shell", Subject: "go test"}
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
	if err := svc.Remember(ctx, RememberRequest{Scope: ScopeGlobal, Tool: "shell", Subject: "go test", Decision: Allow}); !errors.Is(err, ErrRuleStoreUnavailable) {
		t.Fatalf("Remember on nil store error = %v, want ErrRuleStoreUnavailable", err)
	}
	if _, ok, _ := svc.Decide(ctx, Query{Tool: "shell", Subject: "go test"}); ok {
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
	if _, err := New(Mode(255), nil, nil); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("New invalid mode error = %v, want ErrInvalidMode", err)
	}
	if _, err := New(ModePlan, nil, nil); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("New Plan default error = %v, want ErrInvalidMode", err)
	}
	svc := mustPolicy(t, ModeSafe, nil)
	if err := svc.SetDefaultMode(t.Context(), Mode(255)); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("SetDefaultMode invalid mode error = %v, want ErrInvalidMode", err)
	}
	if got, err := svc.Mode(t.Context(), ""); err != nil || got != ModeSafe {
		t.Fatalf("mode after rejected update = (%v, %v), want safe", got, err)
	}
}

type memoryModeStore struct{ states map[string]SessionMode }

func (s *memoryModeStore) GetMode(_ context.Context, sessionID string) (SessionMode, bool, error) {
	state, found := s.states[sessionID]
	return state, found, nil
}

func (s *memoryModeStore) PutMode(_ context.Context, sessionID string, state SessionMode) error {
	s.states[sessionID] = state
	return nil
}

func TestPlanModeIsSessionScopedAndRestoresEntryMode(t *testing.T) {
	modes := &memoryModeStore{states: make(map[string]SessionMode)}
	policy, err := New(ModeBalanced, nil, modes)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := policy.EnterPlanMode(t.Context(), "session-a"); err != nil || !changed {
		t.Fatalf("EnterPlanMode = %v, %v", changed, err)
	}
	if mode, _ := policy.Mode(t.Context(), "session-a"); mode != ModePlan {
		t.Fatalf("session-a mode = %v, want Plan", mode)
	}
	if mode, _ := policy.Mode(t.Context(), "session-b"); mode != ModeBalanced {
		t.Fatalf("session-b mode = %v, want Balanced", mode)
	}
	if err := policy.SetDefaultMode(t.Context(), ModeYolo); err != nil {
		t.Fatal(err)
	}
	if changed, err := policy.EnterPlanMode(t.Context(), "session-a"); err != nil || changed {
		t.Fatalf("second EnterPlanMode = %v, %v, want unchanged", changed, err)
	}
	restored, changed, err := policy.ExitPlanMode(t.Context(), "session-a")
	if err != nil || !changed || restored != ModeBalanced {
		t.Fatalf("ExitPlanMode = %v, %v, %v", restored, changed, err)
	}
	if mode, _ := policy.Mode(t.Context(), "session-a"); mode != ModeBalanced {
		t.Fatalf("restored session-a mode = %v, want captured Balanced", mode)
	}
	if mode, _ := policy.Mode(t.Context(), "session-b"); mode != ModeYolo {
		t.Fatalf("session-b mode = %v, want new default Yolo", mode)
	}
}

func TestSessionModeValidation(t *testing.T) {
	tests := []struct {
		name  string
		state SessionMode
		valid bool
	}{
		{name: "Plan restores safe", state: SessionMode{Mode: ModePlan, RestoreMode: ModeSafe}, valid: true},
		{name: "Plan restores balanced", state: SessionMode{Mode: ModePlan, RestoreMode: ModeBalanced}, valid: true},
		{name: "explicit yolo", state: SessionMode{Mode: ModeYolo}, valid: true},
		{name: "Plan cannot restore Plan", state: SessionMode{Mode: ModePlan, RestoreMode: ModePlan}},
		{name: "unknown mode", state: SessionMode{Mode: Mode(255)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.state.Validate()
			if test.valid && err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if !test.valid && !errors.Is(err, ErrInvalidSessionMode) {
				t.Fatalf("Validate error = %v, want ErrInvalidSessionMode", err)
			}
		})
	}
}

func TestPlanModeRequiresDurableStore(t *testing.T) {
	policy := mustPolicy(t, ModeBalanced, nil)
	if _, err := policy.EnterPlanMode(t.Context(), "session-a"); !errors.Is(err, ErrModeStoreUnavailable) {
		t.Fatalf("EnterPlanMode error = %v, want ErrModeStoreUnavailable", err)
	}
	if _, _, err := policy.ExitPlanMode(t.Context(), "session-a"); !errors.Is(err, ErrModeStoreUnavailable) {
		t.Fatalf("ExitPlanMode error = %v, want ErrModeStoreUnavailable", err)
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

func mustPolicy(t *testing.T, mode Mode, store RuleStore) *RuntimePolicy {
	t.Helper()
	policy, err := New(mode, store, nil)
	if err != nil {
		t.Fatalf("New policy: %v", err)
	}
	return policy
}

func mustRule(t *testing.T, scope Scope, scopeKey, toolName, subject string, decision Decision) Rule {
	t.Helper()
	rule, err := NewRule(scope, scopeKey, toolName, subject, decision)
	if err != nil {
		t.Fatalf("NewRule: %v", err)
	}
	return rule
}
