package workspace

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

func TestListUsesCatalogPort(t *testing.T) {
	catalog := &fakeSkillCatalog{
		skills: []SkillSummary{{Name: "lint", Description: "check code", Scope: SkillScopeProject}},
	}
	c := NewSkills(NewScope("", "", testPaths{}), catalog, nil, nil, nil, nil)

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
	c := NewSkills(NewScope("", "", testPaths{}), nil, nil, nil, nil, nil)
	got, err := c.List(context.Background(), "/repo")
	if err != nil || got != nil {
		t.Fatalf("List = %v, %v; want nil, nil", got, err)
	}
}

func TestManagedSkillsWithoutCuratorReportUnavailable(t *testing.T) {
	c := NewSkills(NewScope("", "", testPaths{}), nil, nil, nil, nil, nil)
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

func TestSkillMutationsPublishOnlyCommittedFilesystemFacts(t *testing.T) {
	curator := &fakeSkillCurator{}
	proposals := &fakeSkillProposals{}
	watcher := &recordingAuthoredWatcher{}
	observations := NewAuthoredWatch(NewScope("", "", testPaths{}), staticWorkspaceInspector{
		resolved: Resolved{Path: "/repo", ProjectRoot: "/repo"},
	}, watcher)
	observation, err := observations.Watch([]string{"/repo"}, []AuthoredResource{AuthoredSkills}, func(AuthoredResource) {})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = observation.Close() }()
	var notices []invalidation.Notice
	c := NewSkills(NewScope("", "", testPaths{}), nil, curator, proposals, observations, func(notice invalidation.Notice) {
		notices = append(notices, notice)
	})

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
	if err := c.RejectProposal(context.Background(), "/repo", ref); err != nil {
		t.Fatal(err)
	}
	if len(notices) != 5 {
		t.Fatalf("notifications = %d, want 5", len(notices))
	}
	for _, notice := range notices {
		if notice.Resource != invalidation.Skills {
			t.Fatalf("notice = %+v, want Skills", notice)
		}
	}
	if len(watcher.accepted) != 5 {
		t.Fatalf("accepted authored changes = %+v, want 5", watcher.accepted)
	}

	curator.archiveErr = errors.New("disk unavailable")
	curator.archiveIdentities = []string{"/skills/partially-moved/SKILL.md"}
	if err := c.Archive(context.Background(), "lint"); err == nil {
		t.Fatal("Archive error = nil, want failure")
	}
	if len(notices) != 6 || len(watcher.accepted) != 6 {
		t.Fatalf("partially committed mutation = notices %d, accepted %d; want 6, 6", len(notices), len(watcher.accepted))
	}
	curator.archiveIdentities = nil
	proposals.approveErr = errors.New("disk unavailable")
	if err := c.ApproveProposal(context.Background(), "/repo", ref); err == nil {
		t.Fatal("ApproveProposal error = nil, want failure")
	}
	proposals.rejectErr = errors.New("disk unavailable")
	if err := c.RejectProposal(context.Background(), "/repo", ref); err == nil {
		t.Fatal("RejectProposal error = nil, want failure")
	}
	if len(notices) != 6 {
		t.Fatalf("uncommitted failure notifications = %d, want 6", len(notices))
	}
}

type fakeSkillCatalog struct {
	cwd    string
	skills []SkillSummary
}

type fakeSkillCurator struct {
	archiveErr        error
	archiveIdentities []string
}

func (f *fakeSkillCurator) List(context.Context) ([]skills.Entry, error) { return nil, nil }
func (f *fakeSkillCurator) Archive(context.Context, string) ([]string, error) {
	if f.archiveErr != nil {
		return f.archiveIdentities, f.archiveErr
	}
	return []string{"/skills/lint/SKILL.md"}, nil
}
func (f *fakeSkillCurator) Restore(context.Context, string) ([]string, error) {
	return []string{"/skills/lint/SKILL.md"}, nil
}

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
