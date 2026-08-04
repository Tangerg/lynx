package knowledge

import "testing"

func TestScopeIsAClosedVocabulary(t *testing.T) {
	for _, scope := range []Scope{ScopeProject, ScopeUser} {
		if err := scope.Validate(); err != nil {
			t.Fatalf("Validate(%q): %v", scope, err)
		}
		if scope.String() != string(scope) {
			t.Fatalf("String(%q) = %q", scope, scope.String())
		}
	}
	for _, scope := range []Scope{"", "workspace", "home"} {
		if err := scope.Validate(); err == nil {
			t.Fatalf("Validate(%q) succeeded", scope)
		}
	}
}
