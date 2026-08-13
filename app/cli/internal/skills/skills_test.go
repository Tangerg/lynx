package skills

import "testing"

const testSkillRevision = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestProposalReferencePreservesImmutableReviewIdentity(t *testing.T) {
	proposal := Proposal{
		Name: "release-checks", Revision: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Scope: UserScope, Description: "Review releases consistently.", Instructions: "Run every release gate.",
		Origin: Requested,
	}
	if err := proposal.Validate(); err != nil {
		t.Fatal(err)
	}
	reference, err := proposal.Reference("/workspace")
	if err != nil {
		t.Fatal(err)
	}
	if reference.Name != proposal.Name || reference.Revision != proposal.Revision || reference.Scope != proposal.Scope {
		t.Fatalf("reference = %+v", reference)
	}
	if proposal.Key() != "user/release-checks@0123456789ab" {
		t.Fatalf("proposal key = %q", proposal.Key())
	}
	proposal.Revision = "changed"
	if err := proposal.Validate(); err == nil {
		t.Fatal("malformed proposal revision was accepted")
	}
}

func TestSkillClosedVocabulariesRejectUnknownValues(t *testing.T) {
	if err := (Discovered{Name: "review", Scope: Scope("global")}).Validate(); err == nil {
		t.Fatal("unknown scope was accepted")
	}
	if err := (Managed{Name: "review", Lifecycle: Lifecycle("stale")}).Validate(); err == nil {
		t.Fatal("unknown lifecycle was accepted")
	}
}

func TestManagedSkillValidatesLifecycleAcknowledgement(t *testing.T) {
	catalog := []Managed{{Name: "review", Lifecycle: Active}}
	if err := ValidateLifecycleAcknowledgement(catalog, "review", Active); err != nil {
		t.Fatalf("active lifecycle acknowledgement: %v", err)
	}
	if err := ValidateLifecycleAcknowledgement(catalog, "review", Archived); err == nil {
		t.Fatal("accepted unchanged lifecycle")
	}
	if err := ValidateLifecycleAcknowledgement(catalog, "missing", Active); err == nil {
		t.Fatal("accepted missing managed skill")
	}
}

func TestProposalReferenceValidatesDecisionAcknowledgement(t *testing.T) {
	reference := ProposalReference{Workspace: "/workspace", Name: "release-checks", Revision: testSkillRevision, Scope: UserScope}
	decided := Proposal{
		Name: reference.Name, Revision: reference.Revision, Scope: reference.Scope,
		Description: "Release safely", Instructions: "Run every gate.",
	}
	if err := reference.ValidateDecisionAcknowledgement([]Proposal{decided}); err == nil {
		t.Fatal("accepted the reviewed proposal as still pending")
	}
	newRevision := decided
	newRevision.Revision = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	if err := reference.ValidateDecisionAcknowledgement([]Proposal{newRevision}); err != nil {
		t.Fatalf("new proposal revision: %v", err)
	}
}
