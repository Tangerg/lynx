package skillauthoring

import (
	"errors"
	"strings"
	"testing"

	skillspec "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

func TestRenderProposalEmitsProvenanceMetadata(t *testing.T) {
	proposal := skills.Proposal{Scope: skills.ScopeUser,
		Name:          "run-project-tests",
		Description:   "How to run the test suite. Use when asked to run tests.",
		Instructions:  "Run `go test ./...` from the module root.",
		Origin:        skills.ProposalOriginMined,
		SourceSession: "ses_1",
	}
	content, err := renderProposal(proposal)
	if err != nil {
		t.Fatal(err)
	}

	front, instructions, err := skillspec.Parse(content)
	if err != nil {
		t.Fatalf("rendered proposal does not parse: %v", err)
	}
	if front.Name != proposal.Name || front.Description != proposal.Description {
		t.Fatalf("frontmatter round-trip mismatch: %+v", front)
	}
	if got := front.Metadata[metadataOrigin]; got != string(skills.ProposalOriginMined) {
		t.Errorf("metadata[%q] = %q, want %q", metadataOrigin, got, skills.ProposalOriginMined)
	}
	if got := front.Metadata[metadataSourceSession]; got != "ses_1" {
		t.Errorf("metadata[%q] = %q, want %q", metadataSourceSession, got, "ses_1")
	}
	if !strings.Contains(instructions, "go test") {
		t.Errorf("instructions round-trip lost the instruction: %q", instructions)
	}
}

func TestRenderProposalEmitsRevisesMarker(t *testing.T) {
	content, err := renderProposal(skills.Proposal{Scope: skills.ScopeUser,
		Name:         "run-project-tests",
		Description:  "A revised version of an existing skill.",
		Instructions: "Run `go test ./...` from the module root.",
		Origin:       skills.ProposalOriginMined,
		Revises:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	front, _, err := skillspec.Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	if front.Metadata[metadataRevises] != metadataTrue {
		t.Fatalf("metadata[%q] = %q, want %q", metadataRevises, front.Metadata[metadataRevises], metadataTrue)
	}
}

func TestRenderProposalOmitsEmptyProvenance(t *testing.T) {
	content, err := renderProposal(skills.Proposal{Scope: skills.ScopeUser,
		Name:         "no-provenance",
		Description:  "A hand-authored proposal carries no provenance.",
		Instructions: "do the thing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "metadata:") {
		t.Fatalf("rendered an empty metadata block:\n%s", content)
	}
	front, _, err := skillspec.Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(front.Metadata) != 0 {
		t.Fatalf("expected no metadata, got %v", front.Metadata)
	}
}

func TestRenderProposalIsDeterministic(t *testing.T) {
	proposal := skills.Proposal{Scope: skills.ScopeUser,
		Name:          "stable",
		Description:   "deterministic render keeps content-addressing stable",
		Instructions:  "step one",
		Origin:        skills.ProposalOriginMined,
		SourceSession: "ses_9",
	}
	first, err := renderProposal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	second, err := renderProposal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("render not deterministic:\n%q\n%q", first, second)
	}
}

func TestRenderProposalBoundsCompleteDocument(t *testing.T) {
	proposal := skills.Proposal{
		Scope: skills.ScopeUser, Name: "rendered-envelope",
		Description:  "Frontmatter must count toward the complete authored document envelope.",
		Instructions: strings.Repeat("x", skills.MaxAuthoredSkillDocumentBytes-1),
	}
	if err := proposal.Validate(); err != nil {
		t.Fatalf("raw proposal should fit the instruction pre-check: %v", err)
	}
	if _, err := renderProposal(proposal); !errors.Is(err, skills.ErrDocumentTooLarge) {
		t.Fatalf("renderProposal error = %v, want ErrDocumentTooLarge", err)
	}
}
