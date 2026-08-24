package skills

import (
	"errors"
	"strings"
	"testing"
)

func TestProposalReferenceBindsExactContent(t *testing.T) {
	content := []byte("proposal bytes")
	ref := NewProposalRef(ScopeProject, "reviewed-skill", content)
	if err := ref.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !ref.Matches(content) || ref.Matches([]byte("different bytes")) {
		t.Fatal("ProposalRef did not bind the exact proposal content")
	}
}

func TestProposalValidatesMeaningAndSafety(t *testing.T) {
	safe := Proposal{
		Scope: ScopeUser, Name: "safe-skill",
		Description:  "A sufficiently descriptive Skill proposal.",
		Instructions: "Inspect the requested files and report the result.",
		Origin:       ProposalOriginRequested,
	}
	if err := safe.Validate(); err != nil {
		t.Fatalf("safe Validate() error = %v", err)
	}
	if issue := safe.SafetyIssue(); issue != ProposalSafe {
		t.Fatalf("safe SafetyIssue() = %v, want ProposalSafe", issue)
	}

	dangerous := safe
	dangerous.Instructions = "run rm -rf /"
	if issue := dangerous.SafetyIssue(); issue != ProposalDangerousInstruction {
		t.Fatalf("dangerous SafetyIssue() = %v, want ProposalDangerousInstruction", issue)
	}

	invalidRef := NewProposalRef(Scope("other"), "safe-skill", nil)
	if err := invalidRef.Validate(); err == nil {
		t.Fatal("invalid proposal scope passed validation")
	}

	oversized := safe
	oversized.Instructions = strings.Repeat("x", MaxAuthoredSkillDocumentBytes+1)
	if err := oversized.Validate(); !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("oversized Validate() error = %v, want ErrDocumentTooLarge", err)
	}
}
