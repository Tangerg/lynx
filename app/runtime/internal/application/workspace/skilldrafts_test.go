package workspace

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

type fakeSkillDrafts struct {
	list     []skills.DraftInfo
	promoted []skills.DraftHandle
	rejected []skills.DraftHandle
}

func (f *fakeSkillDrafts) ListDrafts(context.Context) ([]skills.DraftInfo, error) {
	return f.list, nil
}

func (f *fakeSkillDrafts) Promote(_ context.Context, h skills.DraftHandle) error {
	f.promoted = append(f.promoted, h)
	return nil
}

func (f *fakeSkillDrafts) DiscardDraft(_ context.Context, h skills.DraftHandle) error {
	f.rejected = append(f.rejected, h)
	return nil
}

func TestSkillDraftsUnavailableWithoutStore(t *testing.T) {
	c := NewSkills(NewContext("", "", nil), nil, nil, nil, nil)
	handle := skills.DraftHandle{Name: "x", Revision: "r"}
	if _, err := c.ListSkillDrafts(context.Background()); !errors.Is(err, ErrSkillDraftsUnavailable) {
		t.Fatalf("ListSkillDrafts err = %v, want ErrSkillDraftsUnavailable", err)
	}
	if err := c.PromoteSkillDraft(context.Background(), handle); !errors.Is(err, ErrSkillDraftsUnavailable) {
		t.Fatalf("PromoteSkillDraft err = %v, want ErrSkillDraftsUnavailable", err)
	}
	if err := c.RejectSkillDraft(context.Background(), handle); !errors.Is(err, ErrSkillDraftsUnavailable) {
		t.Fatalf("RejectSkillDraft err = %v, want ErrSkillDraftsUnavailable", err)
	}
}

func TestSkillDraftsDelegateToPort(t *testing.T) {
	handle := skills.DraftHandle{Name: "run-tests", Revision: "abc"}
	fake := &fakeSkillDrafts{list: []skills.DraftInfo{{Handle: handle, CreatedBy: skills.CreatedByAgent}}}
	c := NewSkills(NewContext("", "", nil), nil, nil, fake, nil)

	got, err := c.ListSkillDrafts(context.Background())
	if err != nil || len(got) != 1 || got[0].Handle != handle {
		t.Fatalf("ListSkillDrafts = %+v, %v", got, err)
	}
	if err := c.PromoteSkillDraft(context.Background(), handle); err != nil {
		t.Fatal(err)
	}
	if err := c.RejectSkillDraft(context.Background(), handle); err != nil {
		t.Fatal(err)
	}
	if len(fake.promoted) != 1 || fake.promoted[0] != handle {
		t.Fatalf("promoted = %+v", fake.promoted)
	}
	if len(fake.rejected) != 1 || fake.rejected[0] != handle {
		t.Fatalf("rejected = %+v", fake.rejected)
	}
}
