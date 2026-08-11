package hookpolicy

import "testing"

func TestCatalogEnforcesTrustProjection(t *testing.T) {
	valid := Catalog{ProjectRoot: "/repo", ProjectTrusted: true, Hooks: []Hook{{
		Event: PreToolUse, Matcher: "shell*", Command: "check", Scope: Project, Source: "/repo/.lyra/hooks.json", Active: true,
	}, {Event: Stop, Inject: "done", Scope: Global, Source: "/home/.lyra/hooks.json", Active: true}}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid catalog: %v", err)
	}
	invalid := valid
	invalid.Hooks = append([]Hook(nil), valid.Hooks...)
	invalid.Hooks[0].Active = false
	if err := invalid.Validate(); err == nil {
		t.Fatal("accepted project active state that disagrees with trust")
	}
}
