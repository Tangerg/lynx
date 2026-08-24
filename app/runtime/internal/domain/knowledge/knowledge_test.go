package knowledge

import (
	"errors"
	"strings"
	"testing"
)

func TestScopeIsAClosedVocabulary(t *testing.T) {
	for _, scope := range []Scope{ScopeCWD, ScopeProjectRoot, ScopeHome} {
		if err := scope.Validate(); err != nil {
			t.Fatalf("Validate(%q): %v", scope, err)
		}
		if scope.String() != string(scope) {
			t.Fatalf("String(%q) = %q", scope, scope.String())
		}
	}
	for _, scope := range []Scope{"", "workspace", "project", "user"} {
		if err := scope.Validate(); err == nil {
			t.Fatalf("Validate(%q) succeeded", scope)
		}
	}
}

func TestKnowledgeDocumentEnvelope(t *testing.T) {
	if err := ValidateDocument(strings.Repeat("x", int(MaxDocumentBytes))); err != nil {
		t.Fatalf("exact document boundary: %v", err)
	}
	if err := ValidateDocument(strings.Repeat("x", int(MaxDocumentBytes)+1)); !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("oversized document error = %v, want ErrDocumentTooLarge", err)
	}
}
