package workspace

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

func TestListUsesCatalogPort(t *testing.T) {
	catalog := &fakeSkillCatalog{
		skills: []SkillSummary{{Name: "lint", Description: "check code", Scope: SkillScopeProject}},
	}
	c := NewSkills(NewScope("", "", testPaths{}), catalog, nil, nil, nil)

	got, err := c.List(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if catalog.cwd != "/repo" {
		t.Fatalf("catalog cwd = %q", catalog.cwd)
	}
	if len(got) != 1 || got[0].Name != "lint" {
		t.Fatalf("skills = %+v", got)
	}
}

func TestListWithoutCatalogReturnsNil(t *testing.T) {
	c := NewSkills(NewScope("", "", testPaths{}), nil, nil, nil, nil)
	got, err := c.List(context.Background(), "/repo")
	if err != nil || got != nil {
		t.Fatalf("List = %v, %v; want nil, nil", got, err)
	}
}

func TestManagedSkillsWithoutCuratorReportUnavailable(t *testing.T) {
	c := NewSkills(NewScope("", "", testPaths{}), nil, nil, nil, nil)
	if _, err := c.Managed(context.Background()); !errors.Is(err, ErrSkillLibraryUnavailable) {
		t.Fatalf("Managed err = %v, want ErrSkillLibraryUnavailable", err)
	}
	if err := c.Archive(context.Background(), "lint"); !errors.Is(err, ErrSkillLibraryUnavailable) {
		t.Fatalf("Archive err = %v, want ErrSkillLibraryUnavailable", err)
	}
	if err := c.Restore(context.Background(), "lint"); !errors.Is(err, ErrSkillLibraryUnavailable) {
		t.Fatalf("Restore err = %v, want ErrSkillLibraryUnavailable", err)
	}
}

func TestSkillMutationsNotifyOnlyAfterSuccessfulCommit(t *testing.T) {
	curator := &fakeSkillCurator{}
	proposals := &fakeSkillProposals{}
	notifications := 0
	c := NewSkills(NewScope("", "", testPaths{}), nil, curator, proposals, func(struct{}) { notifications++ })

	if err := c.Archive(context.Background(), "lint"); err != nil {
		t.Fatal(err)
	}
	if err := c.Restore(context.Background(), "lint"); err != nil {
		t.Fatal(err)
	}
	proposal := skills.Proposal{Scope: skills.ScopeProject, Name: "lint", Description: "Lint the current project before final verification.", Instructions: "Run the linter."}
	ref, err := c.SubmitProposal(context.Background(), "/repo", proposal)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ApproveProposal(context.Background(), "/repo", ref); err != nil {
		t.Fatal(err)
	}
	if notifications != 4 {
		t.Fatalf("notifications = %d, want 4", notifications)
	}

	curator.archiveErr = errors.New("disk unavailable")
	if err := c.Archive(context.Background(), "lint"); err == nil {
		t.Fatal("Archive error = nil, want failure")
	}
	proposals.approveErr = errors.New("disk unavailable")
	if err := c.ApproveProposal(context.Background(), "/repo", ref); err == nil {
		t.Fatal("ApproveProposal error = nil, want failure")
	}
	if notifications != 4 {
		t.Fatalf("failed mutation notifications = %d, want 4", notifications)
	}
}

type fakeSkillCatalog struct {
	cwd    string
	skills []SkillSummary
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

func (f *fakeSkillCatalog) List(_ context.Context, cwd string) ([]SkillSummary, error) {
	f.cwd = cwd
	return f.skills, nil
}
