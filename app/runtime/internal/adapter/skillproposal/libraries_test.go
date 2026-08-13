package skillproposal_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/promptsource"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/skillproposal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
)

func TestLibrariesRouteProposalsByScope(t *testing.T) {
	userRoot := filepath.Join(t.TempDir(), "user-skills")
	projectRoot := filepath.Join(t.TempDir(), "project")
	libraries := skillproposal.NewLibraries(skillauthoring.NewStore(userRoot, skills.ScopeUser))

	projectProposal := proposal(skills.ScopeProject, "project-check")
	projectRef, _, err := libraries.SubmitProposal(t.Context(), projectRoot, projectProposal)
	if err != nil {
		t.Fatalf("SubmitProposal(project): %v", err)
	}
	userProposal := proposal(skills.ScopeUser, "personal-check")
	userRef, _, err := libraries.SubmitProposal(t.Context(), projectRoot, userProposal)
	if err != nil {
		t.Fatalf("SubmitProposal(user): %v", err)
	}

	got, err := libraries.ListProposals(t.Context(), projectRoot)
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	if len(got) != 2 || got[0].Ref != projectRef || got[1].Ref != userRef {
		t.Fatalf("ListProposals = %+v; want project then user", got)
	}

	if _, err := libraries.ApproveProposal(t.Context(), projectRoot, projectRef); err != nil {
		t.Fatalf("ApproveProposal(project): %v", err)
	}
	if _, err := libraries.ApproveProposal(t.Context(), projectRoot, userRef); err != nil {
		t.Fatalf("ApproveProposal(user): %v", err)
	}
	assertFile(t, filepath.Join(promptsource.ProjectSkillDir(projectRoot), projectRef.Name, "SKILL.md"))
	assertFile(t, filepath.Join(userRoot, userRef.Name, "SKILL.md"))
}

func TestLibrariesRejectProposalFromItsScopedStore(t *testing.T) {
	userRoot := filepath.Join(t.TempDir(), "user-skills")
	projectRoot := filepath.Join(t.TempDir(), "project")
	libraries := skillproposal.NewLibraries(skillauthoring.NewStore(userRoot, skills.ScopeUser))
	ref, _, err := libraries.SubmitProposal(t.Context(), projectRoot, proposal(skills.ScopeProject, "throwaway"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := libraries.RejectProposal(t.Context(), projectRoot, ref); err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}
	got, err := libraries.ListProposals(t.Context(), projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("rejected proposal still listed: %+v", got)
	}
}

func proposal(scope skills.Scope, name string) skills.Proposal {
	return skills.Proposal{
		Scope:        scope,
		Name:         name,
		Description:  "A reusable workflow with enough detail for Skill validation.",
		Instructions: "Follow the reusable workflow exactly.",
		Origin:       skills.ProposalOriginRequested,
	}
}

func assertFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
}
