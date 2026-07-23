package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

type stubSkillDrafts struct {
	list       []skills.DraftInfo
	promoteErr error
	promoted   []skills.DraftHandle
	rejected   []skills.DraftHandle
}

func (s *stubSkillDrafts) ListDrafts(context.Context) ([]skills.DraftInfo, error) {
	return s.list, nil
}

func (s *stubSkillDrafts) Promote(_ context.Context, h skills.DraftHandle) error {
	if s.promoteErr != nil {
		return s.promoteErr
	}
	s.promoted = append(s.promoted, h)
	return nil
}

func (s *stubSkillDrafts) DiscardDraft(_ context.Context, h skills.DraftHandle) error {
	s.rejected = append(s.rejected, h)
	return nil
}

func TestSkillDraftsHandlersDisabled(t *testing.T) {
	s := newWorkspaceServerWithConfig("", workspaceTestConfig{})
	if _, err := s.ListSkillDrafts(context.Background(), protocol.PageQuery{}); !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("list err = %v, want capability_not_negotiated", err)
	}
	ref := protocol.SkillDraftRef{Name: "x", Revision: "r"}
	if err := s.PromoteSkillDraft(context.Background(), ref); !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("promote err = %v, want capability_not_negotiated", err)
	}
	if err := s.RejectSkillDraft(context.Background(), ref); !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("reject err = %v, want capability_not_negotiated", err)
	}
}

func TestSkillDraftsListMapsWire(t *testing.T) {
	handle := skills.DraftHandle{Name: "run-tests", Revision: "abc123"}
	stub := &stubSkillDrafts{list: []skills.DraftInfo{
		{Handle: handle, Description: "run the tests", CreatedBy: skills.CreatedByAgent, SourceSession: "ses_1"},
	}}
	s := newWorkspaceServerWithConfig("", workspaceTestConfig{Drafts: stub})

	out, err := s.ListSkillDrafts(context.Background(), protocol.PageQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Data) != 1 {
		t.Fatalf("data = %+v", out.Data)
	}
	got := out.Data[0]
	if got.Name != "run-tests" || got.Revision != "abc123" || got.Description != "run the tests" ||
		got.CreatedBy != skills.CreatedByAgent || got.SourceSession != "ses_1" {
		t.Fatalf("wire draft = %+v", got)
	}
}

func TestSkillDraftsPromoteRejectValidateAndDelegate(t *testing.T) {
	stub := &stubSkillDrafts{}
	s := newWorkspaceServerWithConfig("", workspaceTestConfig{Drafts: stub})

	if err := s.PromoteSkillDraft(context.Background(), protocol.SkillDraftRef{Revision: "r"}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("promote missing name → %v, want invalid_params", err)
	}
	if err := s.RejectSkillDraft(context.Background(), protocol.SkillDraftRef{Name: "n"}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("reject missing revision → %v, want invalid_params", err)
	}

	ref := protocol.SkillDraftRef{Name: "run-tests", Revision: "abc123"}
	if err := s.PromoteSkillDraft(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	if err := s.RejectSkillDraft(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	want := skills.DraftHandle{Name: "run-tests", Revision: "abc123"}
	if len(stub.promoted) != 1 || stub.promoted[0] != want {
		t.Fatalf("promoted = %+v", stub.promoted)
	}
	if len(stub.rejected) != 1 || stub.rejected[0] != want {
		t.Fatalf("rejected = %+v", stub.rejected)
	}
}

func TestSkillDraftsPromoteConflictMapsInvalidParams(t *testing.T) {
	stub := &stubSkillDrafts{promoteErr: skills.ErrConflict}
	s := newWorkspaceServerWithConfig("", workspaceTestConfig{Drafts: stub})
	err := s.PromoteSkillDraft(context.Background(), protocol.SkillDraftRef{Name: "n", Revision: "r"})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("conflict → %v, want invalid_params", err)
	}
}
