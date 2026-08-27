package agentexec

import (
	"strings"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/application/workspace"
)

func TestAgentDocumentsPromptAnnotatesEachSource(t *testing.T) {
	out := agentDocumentsPromptForTest(t, []workspace.AgentDocFile{
		{Path: "/a/AGENTS.md", Content: "alpha", Scope: workspace.AgentDocScopeProjectRoot},
		{Path: "/b/AGENTS.md", Content: "beta", Scope: workspace.AgentDocScopeCWD},
	}, agentDocPromptMaxBytes).text

	for _, want := range []string{"<!-- From: /a/AGENTS.md -->", "<!-- From: /b/AGENTS.md -->", "alpha", "beta"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered docs = %q, missing %q", out, want)
		}
	}
	if strings.Index(out, "alpha") > strings.Index(out, "beta") {
		t.Fatalf("docs are out of order: %q", out)
	}
}

func TestAgentDocumentsPromptKeepsMostSpecificFilesWithinBudget(t *testing.T) {
	files := []workspace.AgentDocFile{
		{Path: "/root/AGENTS.md", Content: strings.Repeat("a", 100), Scope: workspace.AgentDocScopeProjectRoot},
		{Path: "/leaf/AGENTS.md", Content: "leaf", Scope: workspace.AgentDocScopeCWD},
	}
	out := agentDocumentsPromptForTest(t, files, 200).text
	if strings.Contains(out, "/root/AGENTS.md") || !strings.Contains(out, "leaf") {
		t.Fatalf("budgeted docs = %q", out)
	}
	prompt := agentDocumentsPromptForTest(t, files, 200)
	if len(prompt.sources) != 1 || prompt.sources[0].Reference != "/leaf/AGENTS.md" {
		t.Fatalf("projected sources = %v", prompt.sources)
	}
	if agentDocumentsPromptForTest(t, nil, agentDocPromptMaxBytes).text != "" ||
		agentDocumentsPromptForTest(t, files, 0).text != "" {
		t.Fatal("empty input or budget must render no prompt text")
	}
}

func agentDocumentsPromptForTest(
	t *testing.T,
	files []workspace.AgentDocFile,
	maxBytes int,
) agentDocumentsPrompt {
	t.Helper()
	prompt, err := newAgentDocumentsPrompt(files, maxBytes)
	if err != nil {
		t.Fatal(err)
	}
	return prompt
}
