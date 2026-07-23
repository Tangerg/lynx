package workspace

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

func TestListSkillsUsesCatalogPort(t *testing.T) {
	catalog := &fakeSkillCatalog{
		skills: []SkillInfo{{Name: "lint", Description: "check code", Scope: "project"}},
	}
	c := NewSkills(NewContext("", "", testPaths{}), catalog, nil, nil, nil)

	got, err := c.ListSkills(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("ListSkills err = %v", err)
	}
	if catalog.cwd != "/repo" {
		t.Fatalf("catalog cwd = %q", catalog.cwd)
	}
	if len(got) != 1 || got[0].Name != "lint" {
		t.Fatalf("skills = %+v", got)
	}
}

func TestListSkillsWithoutCatalogReturnsNil(t *testing.T) {
	c := NewSkills(NewContext("", "", testPaths{}), nil, nil, nil, nil)
	got, err := c.ListSkills(context.Background(), "/repo")
	if err != nil || got != nil {
		t.Fatalf("ListSkills = %v, %v; want nil, nil", got, err)
	}
}

func TestSkillMutationsNotifyOnlyAfterSuccessfulCommit(t *testing.T) {
	curator := &fakeSkillCurator{}
	drafts := &fakeSkillDrafts{}
	notifications := 0
	c := NewSkills(NewContext("", "", testPaths{}), nil, curator, drafts, func(struct{}) { notifications++ })

	if err := c.ArchiveSkill(context.Background(), "lint"); err != nil {
		t.Fatal(err)
	}
	if err := c.RestoreSkill(context.Background(), "lint"); err != nil {
		t.Fatal(err)
	}
	if err := c.PromoteSkillDraft(context.Background(), skills.DraftHandle{Name: "lint", Revision: "r1"}); err != nil {
		t.Fatal(err)
	}
	if notifications != 3 {
		t.Fatalf("notifications = %d, want 3", notifications)
	}

	curator.archiveErr = errors.New("disk unavailable")
	if err := c.ArchiveSkill(context.Background(), "lint"); err == nil {
		t.Fatal("ArchiveSkill error = nil, want failure")
	}
	if notifications != 3 {
		t.Fatalf("failed mutation notifications = %d, want 3", notifications)
	}
}

type fakeSkillCatalog struct {
	cwd    string
	skills []SkillInfo
}

type fakeSkillCurator struct {
	archiveErr error
}

func (f *fakeSkillCurator) List(context.Context) ([]skills.Entry, error) { return nil, nil }
func (f *fakeSkillCurator) Archive(context.Context, string) error        { return f.archiveErr }
func (f *fakeSkillCurator) Restore(context.Context, string) error        { return nil }

type testPaths struct{}

func (testPaths) ResolveExistingDir(path string) (string, error) { return path, nil }
func (testPaths) ResolveInRoot(_, path string) (string, error)   { return path, nil }
func (testPaths) ResolveExistingInRoot(_, path string) (string, error) {
	return path, nil
}

func (f *fakeSkillCatalog) ListSkills(_ context.Context, cwd string) ([]SkillInfo, error) {
	f.cwd = cwd
	return f.skills, nil
}
