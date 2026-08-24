package runtimehost_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func TestPublicResourceDiscoveryCurationAndMutationSurfaces(t *testing.T) {
	harness := newAcceptanceHarness(t)
	baseURL, namespace, meta := harness.runtime.baseURL, harness.namespace, harness.meta
	workspace := protocol.WorkspaceRef{Path: harness.workspace}

	projectSkill := filepath.Join(harness.workspace, ".lyra", "skills", "project-skill")
	userSkill := filepath.Join(harness.home, ".lyra", "skills", "user-skill")
	projectRecipes := filepath.Join(harness.workspace, ".lyra", "recipes")
	for _, directory := range []string{projectSkill, userSkill, projectRecipes} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("create resource fixture directory %q error = %v", directory, err)
		}
	}
	writeFixtureFile(t, filepath.Join(projectSkill, "SKILL.md"), "---\nname: project-skill\ndescription: Project acceptance skill.\n---\nUse the project workflow.\n")
	writeFixtureFile(t, filepath.Join(userSkill, "SKILL.md"), "---\nname: user-skill\ndescription: User acceptance skill.\n---\nUse the user workflow.\n")
	writeFixtureFile(t, filepath.Join(projectRecipes, "review.md"), "---\ndescription: Review a bounded surface.\nargumentHint: '[surface]'\n---\nReview $ARGUMENTS carefully.\n")
	writeFixtureFile(t, filepath.Join(harness.workspace, "AGENTS.md"), "# Acceptance agents\n\nKeep the public boundary explicit.\n")

	discovered := rpcCallWithMeta[*protocol.Page[protocol.Skill]](
		t, baseURL, "skills.discovered.list", protocol.WorkspaceQuery{Workspace: workspace}, meta,
	)
	assertSkill(t, discovered.Data, "project-skill", protocol.SkillScopeProject)
	assertSkill(t, discovered.Data, "user-skill", protocol.SkillScopeUser)
	managed := rpcCallWithMeta[*protocol.Page[protocol.ManagedSkill]](
		t, baseURL, "skills.library.list", struct{}{}, meta,
	)
	if len(managed.Data) != 1 || managed.Data[0].Name != "user-skill" ||
		managed.Data[0].Lifecycle != protocol.SkillLifecycleActive {
		t.Fatalf("skills.library.list = %+v", managed.Data)
	}
	rpcCall[struct{}](
		t, baseURL, "skills.library.archive", protocol.SkillNameRequest{Name: "user-skill"},
		"archive-user-skill", namespace,
	)
	managed = rpcCallWithMeta[*protocol.Page[protocol.ManagedSkill]](
		t, baseURL, "skills.library.list", struct{}{}, meta,
	)
	if len(managed.Data) != 1 || managed.Data[0].Lifecycle != protocol.SkillLifecycleArchived {
		t.Fatalf("archived skills.library.list = %+v", managed.Data)
	}
	discovered = rpcCallWithMeta[*protocol.Page[protocol.Skill]](
		t, baseURL, "skills.discovered.list", protocol.WorkspaceQuery{Workspace: workspace}, meta,
	)
	if findSkill(discovered.Data, "user-skill") != nil {
		t.Fatalf("archived Skill remained discoverable: %+v", discovered.Data)
	}
	rpcCall[struct{}](
		t, baseURL, "skills.library.restore", protocol.SkillNameRequest{Name: "user-skill"},
		"restore-user-skill", namespace,
	)
	discovered = rpcCallWithMeta[*protocol.Page[protocol.Skill]](
		t, baseURL, "skills.discovered.list", protocol.WorkspaceQuery{Workspace: workspace}, meta,
	)
	assertSkill(t, discovered.Data, "user-skill", protocol.SkillScopeUser)

	recipes := rpcCallWithMeta[*protocol.Page[protocol.Recipe]](
		t, baseURL, "recipes.list", protocol.WorkspaceQuery{Workspace: workspace}, meta,
	)
	if len(recipes.Data) != 1 || recipes.Data[0].Name != "review" ||
		recipes.Data[0].Scope != protocol.RecipeScopeProject ||
		recipes.Data[0].ArgumentHint != "[surface]" ||
		recipes.Data[0].Body != "Review $ARGUMENTS carefully." {
		t.Fatalf("recipes.list = %+v", recipes.Data)
	}
	agentDocs := rpcCallWithMeta[*protocol.Page[protocol.AgentDoc]](
		t, baseURL, "agentDocs.list", protocol.WorkspaceQuery{Workspace: workspace}, meta,
	)
	if len(agentDocs.Data) != 1 || agentDocs.Data[0].Path != filepath.Join(harness.workspace, "AGENTS.md") ||
		agentDocs.Data[0].Title != "Acceptance agents" || agentDocs.Data[0].Scope != protocol.AgentDocScopeCWD {
		t.Fatalf("agentDocs.list = %+v", agentDocs.Data)
	}

	homeKnowledge := rpcCallWithMeta[*protocol.KnowledgeEntry](
		t, baseURL, "knowledge.get", protocol.GetKnowledgeRequest{Scope: protocol.KnowledgeScopeHome}, meta,
	)
	homeKnowledge = rpcCall[*protocol.KnowledgeEntry](
		t, baseURL, "knowledge.update", protocol.UpdateKnowledgeRequest{
			Scope: protocol.KnowledgeScopeHome, ExpectedRevision: homeKnowledge.Revision,
			Content: "Prefer explicit public contracts.\n",
		}, "update-home-knowledge", namespace,
	)
	cwdKnowledge := rpcCallWithMeta[*protocol.KnowledgeEntry](
		t, baseURL, "knowledge.get", protocol.GetKnowledgeRequest{
			Scope: protocol.KnowledgeScopeCWD, Workspace: &workspace,
		}, meta,
	)
	cwdKnowledge = rpcCall[*protocol.KnowledgeEntry](
		t, baseURL, "knowledge.update", protocol.UpdateKnowledgeRequest{
			Scope: protocol.KnowledgeScopeCWD, Workspace: &workspace,
			ExpectedRevision: cwdKnowledge.Revision, Content: "Keep workspace facts local.\n",
		}, "update-cwd-knowledge", namespace,
	)
	knowledge := rpcCallWithMeta[*protocol.Page[protocol.KnowledgeEntry]](
		t, baseURL, "knowledge.list", protocol.WorkspaceQuery{Workspace: workspace}, meta,
	)
	if len(knowledge.Data) != 2 || homeKnowledge.Content == "" || cwdKnowledge.Content == "" ||
		!containsKnowledge(knowledge.Data, *homeKnowledge) || !containsKnowledge(knowledge.Data, *cwdKnowledge) {
		t.Fatalf("knowledge.list = %+v", knowledge.Data)
	}

	userMemory := rpcCall[*protocol.AgentMemoryItem](
		t, baseURL, "agentMemory.add", protocol.AgentMemoryAddRequest{
			Scope: protocol.AgentMemoryScopeUser, Content: "Prefer bounded public APIs.",
		}, "add-user-memory", namespace,
	)
	projectMemory := rpcCall[*protocol.AgentMemoryItem](
		t, baseURL, "agentMemory.add", protocol.AgentMemoryAddRequest{
			Scope: protocol.AgentMemoryScopeProject, Workspace: &workspace,
			Content: "The workspace uses Lyra Protocol.",
		}, "add-project-memory", namespace,
	)
	pinned := true
	updatedContent := "Prefer bounded and typed public APIs."
	userMemory = rpcCall[*protocol.AgentMemoryItem](
		t, baseURL, "agentMemory.update", protocol.AgentMemoryUpdateRequest{
			ID: userMemory.ID, Content: &updatedContent, Pinned: &pinned,
		}, "update-user-memory", namespace,
	)
	if !userMemory.Pinned || userMemory.Content != updatedContent || userMemory.Status != protocol.AgentMemoryStatusActive {
		t.Fatalf("agentMemory.update = %+v", userMemory)
	}
	userMemories := rpcCallWithMeta[*protocol.AgentMemoryList](
		t, baseURL, "agentMemory.list", protocol.AgentMemoryListRequest{Scope: protocol.AgentMemoryScopeUser}, meta,
	)
	projectMemories := rpcCallWithMeta[*protocol.AgentMemoryList](
		t, baseURL, "agentMemory.list", protocol.AgentMemoryListRequest{
			Scope: protocol.AgentMemoryScopeProject, Workspace: &workspace,
		}, meta,
	)
	if len(userMemories.Items) != 1 || userMemories.Items[0].ID != userMemory.ID ||
		len(projectMemories.Items) != 1 || projectMemories.Items[0].ID != projectMemory.ID {
		t.Fatalf("agentMemory.list user=%+v project=%+v", userMemories.Items, projectMemories.Items)
	}
	rpcCall[struct{}](
		t, baseURL, "agentMemory.delete", protocol.AgentMemoryItemRequest{ID: projectMemory.ID},
		"delete-project-memory", namespace,
	)
	projectMemories = rpcCallWithMeta[*protocol.AgentMemoryList](
		t, baseURL, "agentMemory.list", protocol.AgentMemoryListRequest{
			Scope: protocol.AgentMemoryScopeProject, Workspace: &workspace,
		}, meta,
	)
	if len(projectMemories.Items) != 0 {
		t.Fatalf("agentMemory.list after delete = %+v", projectMemories.Items)
	}

	assertSkillProposalReview(t, harness, workspace, "SCENARIO_PROPOSE_SKILL", "proposed-skill", true)
	assertSkillProposalReview(t, harness, workspace, "SCENARIO_REJECT_SKILL", "rejected-skill", false)
}

func assertSkillProposalReview(
	t *testing.T,
	harness acceptanceHarness,
	workspace protocol.WorkspaceRef,
	scenario string,
	wantedName string,
	approve bool,
) {
	t.Helper()
	session := createRunSession(t, harness.runtime.baseURL, harness.namespace, wantedName)
	ack, events := rpcRunStream[*protocol.StartRunResponse](
		t, harness.runtime.baseURL, "runs.start", protocol.StartRunRequest{
			SessionID: session.ID,
			Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: scenario}},
			Provider:  "openai-compatible", Model: "test-model",
		}, harness.meta, "run-"+wantedName, harness.namespace,
	)
	assertCompletedRunStream(t, ack.RunID, events)
	proposals := rpcCallWithMeta[*protocol.Page[protocol.SkillProposal]](
		t, harness.runtime.baseURL, "skills.proposals.list",
		protocol.WorkspaceQuery{Workspace: workspace}, harness.meta,
	)
	if len(proposals.Data) != 1 || proposals.Data[0].Name != wantedName || proposals.Data[0].Revision == "" {
		t.Fatalf("skills.proposals.list = %+v", proposals.Data)
	}
	proposal := proposals.Data[0]
	method, key := "skills.proposals.reject", "reject-"+wantedName
	if approve {
		method, key = "skills.proposals.approve", "approve-"+wantedName
	}
	rpcCall[struct{}](
		t, harness.runtime.baseURL, method, protocol.SkillProposalRef{
			Workspace: workspace, Name: proposal.Name, Revision: proposal.Revision, Scope: proposal.Scope,
		}, key, harness.namespace,
	)
	proposals = rpcCallWithMeta[*protocol.Page[protocol.SkillProposal]](
		t, harness.runtime.baseURL, "skills.proposals.list",
		protocol.WorkspaceQuery{Workspace: workspace}, harness.meta,
	)
	if len(proposals.Data) != 0 {
		t.Fatalf("%s left pending proposals: %+v", method, proposals.Data)
	}
	discovered := rpcCallWithMeta[*protocol.Page[protocol.Skill]](
		t, harness.runtime.baseURL, "skills.discovered.list",
		protocol.WorkspaceQuery{Workspace: workspace}, harness.meta,
	)
	found := findSkill(discovered.Data, wantedName)
	if approve && (found == nil || found.Scope != protocol.SkillScopeProject) {
		t.Fatalf("approved Skill not discovered: %+v", discovered.Data)
	}
	if !approve && found != nil {
		t.Fatalf("rejected Skill became discoverable: %+v", discovered.Data)
	}
}

func assertSkill(t *testing.T, values []protocol.Skill, name string, scope protocol.SkillScope) {
	t.Helper()
	found := findSkill(values, name)
	if found == nil || found.Scope != scope {
		t.Fatalf("Skill %q/%q not found in %+v", name, scope, values)
	}
}

func findSkill(values []protocol.Skill, name string) *protocol.Skill {
	for index := range values {
		if values[index].Name == name {
			return &values[index]
		}
	}
	return nil
}

func containsKnowledge(values []protocol.KnowledgeEntry, wanted protocol.KnowledgeEntry) bool {
	for _, value := range values {
		if value.Scope == wanted.Scope && value.Revision == wanted.Revision && value.Content == wanted.Content {
			return true
		}
	}
	return false
}
