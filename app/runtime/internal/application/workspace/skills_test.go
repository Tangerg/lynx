package workspace

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

func TestListSkillsUsesCatalogPort(t *testing.T) {
	catalog := &fakeSkillCatalog{
		skills: []skills.Info{{Name: "lint", Description: "check code", Scope: "project"}},
	}
	c := New(Config{Paths: testPaths{}, Skills: catalog})

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
	c := New(Config{Paths: testPaths{}})
	got, err := c.ListSkills(context.Background(), "/repo")
	if err != nil || got != nil {
		t.Fatalf("ListSkills = %v, %v; want nil, nil", got, err)
	}
}

type fakeSkillCatalog struct {
	cwd    string
	skills []skills.Info
}

type testPaths struct{}

func (testPaths) ResolveExistingDir(path string) (string, error) { return path, nil }
func (testPaths) ResolveInRoot(_, path string) (string, error)   { return path, nil }
func (testPaths) ResolveExistingInRoot(_, path string) (string, error) {
	return path, nil
}

func (f *fakeSkillCatalog) ListSkills(_ context.Context, cwd string) ([]skills.Info, error) {
	f.cwd = cwd
	return f.skills, nil
}
