package approval

import (
	"context"
	"testing"
)

// White-box tests: the matching/precedence helpers are the heart of the rule
// engine, so they're exercised directly alongside the Service round-trips.

func TestModeGetSet(t *testing.T) {
	svc := New(ModeYolo, nil)
	if m, _ := svc.GetMode(context.Background()); m != ModeYolo {
		t.Fatalf("initial mode = %v, want Yolo", m)
	}
	if err := svc.SetMode(context.Background(), ModeBalanced); err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	if m, _ := svc.GetMode(context.Background()); m != ModeBalanced {
		t.Fatalf("mode after set = %v, want Balanced", m)
	}
}

func TestSubjectOf(t *testing.T) {
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
		if got := subjectOf(c.tool, c.args); got != c.want {
			t.Errorf("subjectOf(%q,%q) = %q, want %q", c.tool, c.args, got, c.want)
		}
	}
}

func TestSubjectMatches(t *testing.T) {
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
		if got := subjectMatches(c.pattern, c.subject); got != c.want {
			t.Errorf("subjectMatches(%q,%q) = %v, want %v", c.pattern, c.subject, got, c.want)
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
	if d, ok := decide(rules, q); !ok || d != Deny {
		t.Fatalf("exact deny over broad allow = (%v,%v), want (deny,true)", d, ok)
	}

	// A global deny vs a session allow (both whole-tool) → session allow wins.
	rules = []Rule{
		{Scope: ScopeGlobal, Tool: "shell", Subject: "", Decision: Deny},
		{Scope: ScopeSession, Tool: "shell", Subject: "", Decision: Allow},
	}
	if d, ok := decide(rules, q); !ok || d != Allow {
		t.Fatalf("session allow over global deny = (%v,%v), want (allow,true)", d, ok)
	}

	// Wrong tool / no rules → miss.
	if _, ok := decide([]Rule{{Scope: ScopeSession, Tool: "write", Decision: Allow}}, q); ok {
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
	if d, ok := decide(rules, q); !ok || d != Deny {
		t.Fatalf("conflict = (%v,%v), want (deny,true)", d, ok)
	}
}

// TestServiceRememberDecide: a remembered shell command auto-resolves a matching
// future call; a different command still misses (subject granularity).
func TestServiceRememberDecide(t *testing.T) {
	ctx := context.Background()
	svc := New(ModeSafe, NewMemoryStore())
	build := `{"command":"npm run build"}`
	_ = svc.Remember(ctx, RememberRequest{
		Scope: ScopeSession, SessionID: "s1", Tool: "shell", Arguments: build, Decision: Allow,
	})

	if d, ok, _ := svc.Decide(ctx, Query{SessionID: "s1", Tool: "shell", Arguments: build}); !ok || d != Allow {
		t.Fatalf("matching call = (%v,%v), want (allow,true)", d, ok)
	}
	// A different command isn't covered by the remembered one.
	if _, ok, _ := svc.Decide(ctx, Query{SessionID: "s1", Tool: "shell", Arguments: `{"command":"rm -rf /"}`}); ok {
		t.Fatal("a remembered `npm run build` rule matched `rm -rf /`")
	}
}

// TestServiceScopeVisibilityAndForget: a project rule is invisible from another
// dir; Forget(id) removes it.
func TestServiceScopeVisibilityAndForget(t *testing.T) {
	ctx := context.Background()
	svc := New(ModeSafe, NewMemoryStore())
	_ = svc.Remember(ctx, RememberRequest{
		Scope: ScopeProject, ProjectDir: "/proj/a", Tool: "write", Arguments: `{"file_path":"x"}`, Decision: Allow,
	})

	q := Query{SessionID: "s1", ProjectDir: "/proj/a", Tool: "write", Arguments: `{"file_path":"x"}`}
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
	svc := New(ModeSafe, NewMemoryStore())
	_ = svc.Remember(ctx, RememberRequest{
		Scope: ScopeProject, ProjectDir: "", Tool: "shell", Arguments: `{}`, Decision: Allow,
	})
	if rules, _ := svc.Rules(ctx, "s1", ""); len(rules) != 0 {
		t.Fatalf("unkeyable project rule stored: %+v", rules)
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
