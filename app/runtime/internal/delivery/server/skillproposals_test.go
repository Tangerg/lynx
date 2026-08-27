package server

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/domain/skills"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

type stubSkillProposals struct {
	list       []skills.ProposalReview
	approveErr error
	approved   []skills.ProposalRef
	rejected   []skills.ProposalRef
	root       string
}

func (s *stubSkillProposals) SubmitProposal(context.Context, string, skills.Proposal) (skills.ProposalRef, []string, error) {
	panic("unexpected SubmitProposal")
}

func (s *stubSkillProposals) ListProposals(_ context.Context, root string) ([]skills.ProposalReview, error) {
	s.root = root
	return s.list, nil
}

func (s *stubSkillProposals) ApproveProposal(_ context.Context, root string, ref skills.ProposalRef) ([]string, error) {
	s.root = root
	if s.approveErr != nil {
		return nil, s.approveErr
	}
	s.approved = append(s.approved, ref)
	return []string{filepath.Join(root, "run-tests", "SKILL.md")}, nil
}

func (s *stubSkillProposals) RejectProposal(_ context.Context, root string, ref skills.ProposalRef) ([]string, error) {
	s.root = root
	s.rejected = append(s.rejected, ref)
	return []string{filepath.Join(root, "_proposals", ref.Name, "SKILL.md")}, nil
}

func TestSkillProposalHandlersDisabled(t *testing.T) {
	s := newWorkspaceServerWithConfig("", workspaceTestConfig{})
	query := protocol.WorkspaceQuery{}
	if _, err := s.ListSkillProposals(t.Context(), query); !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("list err = %v, want capability_not_negotiated", err)
	}
	ref := wireProposalRef("", skills.NewProposalRef(skills.ScopeProject, "run-tests", []byte("content")))
	if err := s.ApproveSkillProposal(t.Context(), ref); !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("approve err = %v, want capability_not_negotiated", err)
	}
	if err := s.RejectSkillProposal(t.Context(), ref); !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("reject err = %v, want capability_not_negotiated", err)
	}
}

func TestSkillProposalListMapsCompleteReviewContent(t *testing.T) {
	root := t.TempDir()
	ref := skills.NewProposalRef(skills.ScopeProject, "run-tests", []byte("content"))
	stub := &stubSkillProposals{list: []skills.ProposalReview{{
		Ref:           ref,
		Description:   "Run the project tests before final verification.",
		Instructions:  "Run `go test ./...` from the module root.",
		Origin:        skills.ProposalOriginRequested,
		SourceSession: "ses_1",
		Revises:       true,
	}}}
	s := newWorkspaceServerWithConfig(root, workspaceTestConfig{Proposals: stub})

	out, err := s.ListSkillProposals(t.Context(), protocol.WorkspaceQuery{Workspace: protocol.WorkspaceRef{Path: root}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Data) != 1 {
		t.Fatalf("data = %+v", out.Data)
	}
	got := out.Data[0]
	if got.Name != ref.Name || got.Revision != ref.Revision || got.Scope != protocol.SkillScopeProject ||
		got.Description == "" || got.Instructions == "" || got.Origin != protocol.SkillProposalOriginRequested ||
		got.SourceSession != "ses_1" || !got.Revises {
		t.Fatalf("wire proposal = %+v", got)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if stub.root != resolvedRoot {
		t.Fatalf("resolved root = %q, want %q", stub.root, resolvedRoot)
	}
}

func TestSkillProposalApproveRejectValidateAndDelegate(t *testing.T) {
	root := t.TempDir()
	stub := &stubSkillProposals{}
	s := newWorkspaceServerWithConfig(root, workspaceTestConfig{Proposals: stub})
	ref := skills.NewProposalRef(skills.ScopeUser, "run-tests", []byte("content"))
	wire := wireProposalRef(root, ref)

	invalid := wire
	invalid.Revision = "not-a-digest"
	if err := s.ApproveSkillProposal(t.Context(), invalid); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("invalid revision → %v, want invalid_params", err)
	}
	if err := s.ApproveSkillProposal(t.Context(), wire); err != nil {
		t.Fatal(err)
	}
	if err := s.RejectSkillProposal(t.Context(), wire); err != nil {
		t.Fatal(err)
	}
	if len(stub.approved) != 1 || stub.approved[0] != ref {
		t.Fatalf("approved = %+v", stub.approved)
	}
	if len(stub.rejected) != 1 || stub.rejected[0] != ref {
		t.Fatalf("rejected = %+v", stub.rejected)
	}
}

func TestSkillProposalApproveConflictMapsInvalidParams(t *testing.T) {
	root := t.TempDir()
	stub := &stubSkillProposals{approveErr: skills.ErrConflict}
	s := newWorkspaceServerWithConfig(root, workspaceTestConfig{Proposals: stub})
	err := s.ApproveSkillProposal(t.Context(), wireProposalRef(root, skills.NewProposalRef(skills.ScopeProject, "run-tests", []byte("content"))))
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("conflict → %v, want invalid_params", err)
	}
}

func wireProposalRef(root string, ref skills.ProposalRef) protocol.SkillProposalRef {
	scope, _ := presentSkillProposalScope(ref.Scope)
	return protocol.SkillProposalRef{
		Workspace: protocol.WorkspaceRef{Path: root},
		Name:      ref.Name,
		Revision:  ref.Revision,
		Scope:     scope,
	}
}
