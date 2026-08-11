package skills

import "testing"

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
