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
	list       []skills.ProposalReview
	approved   []skills.ProposalRef
	rejected   []skills.ProposalRef
	submitErr  error
	approveErr error
	rejectErr  error
}

func (f *fakeSkillProposals) SubmitProposal(_ context.Context, projectRoot string, proposal skills.Proposal) (skills.ProposalRef, []string, error) {
	f.root = projectRoot
	f.proposal = proposal
	if f.submitErr != nil {
		return skills.ProposalRef{}, nil, f.submitErr
	}
	return skills.NewProposalRef(proposal.Scope, proposal.Name, []byte(proposal.Instructions)), []string{"/repo/.lyra/skills/_proposals/skill-name/SKILL.md"}, nil
}

func (f *fakeSkillProposals) ListProposals(_ context.Context, projectRoot string) ([]skills.ProposalReview, error) {
	f.root = projectRoot
	return f.list, nil
}

func (f *fakeSkillProposals) ApproveProposal(_ context.Context, projectRoot string, ref skills.ProposalRef) ([]string, error) {
	f.root = projectRoot
	if f.approveErr != nil {
		return nil, f.approveErr
	}
	f.approved = append(f.approved, ref)
	return []string{"/repo/.lyra/skills/run-tests/SKILL.md"}, nil
}

func (f *fakeSkillProposals) RejectProposal(_ context.Context, projectRoot string, ref skills.ProposalRef) ([]string, error) {
	f.root = projectRoot
	if f.rejectErr != nil {
		return nil, f.rejectErr
	}
	f.rejected = append(f.rejected, ref)
	return []string{"/repo/.lyra/skills/_proposals/skill-name/SKILL.md"}, nil
}

func TestSkillProposalsUnavailableWithoutStore(t *testing.T) {
	c := NewSkills(NewScope("", "", testPaths{}), nil, nil, nil, nil, nil)
	ref := skills.NewProposalRef(skills.ScopeProject, "run-tests", []byte("content"))
	proposal := skills.Proposal{Scope: skills.ScopeProject, Name: "run-tests", Description: "Run the project tests when verification is requested.", Instructions: "Run the tests."}

	if _, err := c.SubmitProposal(t.Context(), "/repo", proposal); !errors.Is(err, ErrSkillProposalsUnavailable) {
		t.Fatalf("SubmitProposal err = %v, want ErrSkillProposalsUnavailable", err)
	}
	if _, err := c.Proposals(t.Context(), "/repo"); !errors.Is(err, ErrSkillProposalsUnavailable) {
		t.Fatalf("Proposals err = %v, want ErrSkillProposalsUnavailable", err)
	}
	if err := c.ApproveProposal(t.Context(), "/repo", ref); !errors.Is(err, ErrSkillProposalsUnavailable) {
		t.Fatalf("ApproveProposal err = %v, want ErrSkillProposalsUnavailable", err)
	}
	if err := c.RejectProposal(t.Context(), "/repo", ref); !errors.Is(err, ErrSkillProposalsUnavailable) {
		t.Fatalf("RejectProposal err = %v, want ErrSkillProposalsUnavailable", err)
	}
}

func TestSkillProposalsResolveWorkspaceAndDelegate(t *testing.T) {
	proposal := skills.Proposal{Scope: skills.ScopeProject, Name: "run-tests", Description: "Run the project tests when verification is requested.", Instructions: "Run the tests."}
	ref := skills.NewProposalRef(proposal.Scope, proposal.Name, []byte(proposal.Instructions))
	fake := &fakeSkillProposals{list: []skills.ProposalReview{{Ref: ref, Description: proposal.Description, Instructions: proposal.Instructions}}}
	c := NewSkills(NewScope("", "", testPaths{}), nil, nil, fake, nil, nil)

	gotRef, err := c.SubmitProposal(t.Context(), "/repo", proposal)
	if err != nil || gotRef != ref {
		t.Fatalf("SubmitProposal = %+v, %v; want %+v", gotRef, err, ref)
	}
	got, err := c.Proposals(t.Context(), "/repo")
	if err != nil || len(got) != 1 || got[0].Ref != ref {
		t.Fatalf("Proposals = %+v, %v", got, err)
	}
	if err := c.ApproveProposal(t.Context(), "/repo", ref); err != nil {
		t.Fatal(err)
	}
	if err := c.RejectProposal(t.Context(), "/repo", ref); err != nil {
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
