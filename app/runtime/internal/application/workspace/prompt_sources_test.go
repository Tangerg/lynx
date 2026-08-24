package workspace

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestAuthoredPromptDocumentContract(t *testing.T) {
	exact := []byte(strings.Repeat("a", MaxAuthoredPromptDocumentBytes))
	if err := ValidateAuthoredPromptDocument(exact); err != nil {
		t.Fatalf("exact boundary error = %v", err)
	}
	if err := ValidateAuthoredPromptDocument(append(exact, 'a')); !errors.Is(err, ErrPromptSourceTooLarge) {
		t.Fatalf("oversized error = %v, want ErrPromptSourceTooLarge", err)
	}
	if err := ValidateAuthoredPromptDocument([]byte{'a', 0xff}); !errors.Is(err, ErrInvalidPromptSource) {
		t.Fatalf("invalid UTF-8 error = %v, want ErrInvalidPromptSource", err)
	}
}

func TestAgentDocumentCascadeContract(t *testing.T) {
	document := strings.Repeat("a", MaxAuthoredPromptDocumentBytes)
	exact := make([]AgentDocFile, MaxAgentDocumentCascadeBytes/MaxAuthoredPromptDocumentBytes)
	for index := range exact {
		exact[index] = AgentDocFile{
			Path: fmt.Sprintf("/repo/%d/AGENTS.md", index), Content: document,
			Scope: AgentDocScopeProjectRoot,
		}
	}
	if err := ValidateAgentDocumentCascade(exact); err != nil {
		t.Fatalf("exact aggregate boundary error = %v", err)
	}
	if err := ValidateAgentDocumentCascade(append(exact, AgentDocFile{
		Path: "/repo/leaf/AGENTS.md", Content: "x", Scope: AgentDocScopeCWD,
	})); !errors.Is(err, ErrPromptSourceTooLarge) {
		t.Fatalf("aggregate overflow error = %v, want ErrPromptSourceTooLarge", err)
	}

	overfull := make([]AgentDocFile, MaxAgentDocumentsPerCascade+1)
	for index := range overfull {
		overfull[index] = AgentDocFile{
			Path: fmt.Sprintf("/repo/%d/AGENTS.md", index), Content: "x",
			Scope: AgentDocScopeProjectRoot,
		}
	}
	if err := ValidateAgentDocumentCascade(overfull); !errors.Is(err, ErrPromptSourceTooLarge) {
		t.Fatalf("overfull error = %v, want ErrPromptSourceTooLarge", err)
	}
}

func TestRecipeCascadeContract(t *testing.T) {
	document := strings.Repeat("r", MaxAuthoredPromptDocumentBytes)
	exact := make([]Recipe, MaxRecipeCascadeBytes/MaxAuthoredPromptDocumentBytes)
	for index := range exact {
		exact[index] = Recipe{
			Name: fmt.Sprintf("recipe-%d", index), Body: document,
			Scope: RecipeScopeProject, Source: fmt.Sprintf("/repo/recipe-%d.md", index),
		}
	}
	if err := ValidateRecipeCascade(exact); err != nil {
		t.Fatalf("exact aggregate boundary error = %v", err)
	}
	if err := ValidateRecipeCascade(append(exact, Recipe{
		Name: "overflow", Body: "x", Scope: RecipeScopeProject, Source: "/repo/overflow.md",
	})); !errors.Is(err, ErrPromptSourceTooLarge) {
		t.Fatalf("aggregate overflow error = %v, want ErrPromptSourceTooLarge", err)
	}

	overfull := make([]Recipe, MaxRecipesPerScope+1)
	for index := range overfull {
		overfull[index] = Recipe{
			Name: fmt.Sprintf("recipe-%d", index), Body: "x",
			Scope: RecipeScopeGlobal, Source: fmt.Sprintf("/home/recipe-%d.md", index),
		}
	}
	if err := ValidateRecipeCascade(overfull); !errors.Is(err, ErrPromptSourceTooLarge) {
		t.Fatalf("overfull error = %v, want ErrPromptSourceTooLarge", err)
	}
}
