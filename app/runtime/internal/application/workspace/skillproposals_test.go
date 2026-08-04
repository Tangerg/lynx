package workspace

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

type fakeSkillProposals struct {
	root       string
	proposal   skills.Proposal
	list       []skills.ProposalInfo
	approved   []skills.ProposalRef
	rejected   []skills.ProposalRef
	submitErr  error
	approveErr error
	rejectErr  error
}

func (f *fakeSkillProposals) SubmitProposal(_ context.Context, projectRoot string, proposal skills.Proposal) (skills.ProposalRef, error) {
	f.root = projectRoot
	f.proposal = proposal
	if f.submitErr != nil {
		return skills.ProposalRef{}, f.submitErr
	}
	return skills.NewProposalRef(proposal.Scope, proposal.Name, []byte(proposal.Instructions)), nil
}

func (f *fakeSkillProposals) ListProposals(_ context.Context, projectRoot string) ([]skills.ProposalInfo, error) {
	f.root = projectRoot
	return f.list, nil
}

func (f *fakeSkillProposals) ApproveProposal(_ context.Context, projectRoot string, ref skills.ProposalRef) error {
	f.root = projectRoot
	if f.approveErr != nil {
		return f.approveErr
	}
	f.approved = append(f.approved, ref)
	return nil
}

func (f *fakeSkillProposals) RejectProposal(_ context.Context, projectRoot string, ref skills.ProposalRef) error {
	f.root = projectRoot
	if f.rejectErr != nil {
		return f.rejectErr
	}
	f.rejected = append(f.rejected, ref)
	return nil
}

func TestSkillProposalsUnavailableWithoutStore(t *testing.T) {
	c := NewSkills(NewContext("", "", testPaths{}), nil, nil, nil, nil)
	ref := skills.NewProposalRef(skills.ScopeProject, "run-tests", []byte("content"))
	proposal := skills.Proposal{Scope: skills.ScopeProject, Name: "run-tests", Description: "Run the project tests when verification is requested.", Instructions: "Run the tests."}

	if _, err := c.SubmitSkillProposal(t.Context(), "/repo", proposal); !errors.Is(err, ErrSkillProposalsUnavailable) {
		t.Fatalf("SubmitSkillProposal err = %v, want ErrSkillProposalsUnavailable", err)
	}
	if _, err := c.ListSkillProposals(t.Context(), "/repo"); !errors.Is(err, ErrSkillProposalsUnavailable) {
		t.Fatalf("ListSkillProposals err = %v, want ErrSkillProposalsUnavailable", err)
	}
	if err := c.ApproveSkillProposal(t.Context(), "/repo", ref); !errors.Is(err, ErrSkillProposalsUnavailable) {
		t.Fatalf("ApproveSkillProposal err = %v, want ErrSkillProposalsUnavailable", err)
	}
	if err := c.RejectSkillProposal(t.Context(), "/repo", ref); !errors.Is(err, ErrSkillProposalsUnavailable) {
		t.Fatalf("RejectSkillProposal err = %v, want ErrSkillProposalsUnavailable", err)
	}
}

func TestSkillProposalsResolveWorkspaceAndDelegate(t *testing.T) {
	proposal := skills.Proposal{Scope: skills.ScopeProject, Name: "run-tests", Description: "Run the project tests when verification is requested.", Instructions: "Run the tests."}
	ref := skills.NewProposalRef(proposal.Scope, proposal.Name, []byte(proposal.Instructions))
	fake := &fakeSkillProposals{list: []skills.ProposalInfo{{Ref: ref, Description: proposal.Description, Instructions: proposal.Instructions}}}
	c := NewSkills(NewContext("", "", testPaths{}), nil, nil, fake, nil)

	gotRef, err := c.SubmitSkillProposal(t.Context(), "/repo", proposal)
	if err != nil || gotRef != ref {
		t.Fatalf("SubmitSkillProposal = %+v, %v; want %+v", gotRef, err, ref)
	}
	got, err := c.ListSkillProposals(t.Context(), "/repo")
	if err != nil || len(got) != 1 || got[0].Ref != ref {
		t.Fatalf("ListSkillProposals = %+v, %v", got, err)
	}
	if err := c.ApproveSkillProposal(t.Context(), "/repo", ref); err != nil {
		t.Fatal(err)
	}
	if err := c.RejectSkillProposal(t.Context(), "/repo", ref); err != nil {
		t.Fatal(err)
	}
	if fake.root != "/repo" || fake.proposal != proposal {
		t.Fatalf("delegated root/proposal = %q, %+v", fake.root, fake.proposal)
	}
	if len(fake.approved) != 1 || fake.approved[0] != ref {
		t.Fatalf("approved = %+v", fake.approved)
	}
	if len(fake.rejected) != 1 || fake.rejected[0] != ref {
		t.Fatalf("rejected = %+v", fake.rejected)
	}
}
