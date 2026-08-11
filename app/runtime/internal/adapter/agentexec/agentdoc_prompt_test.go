package agentexec

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
)

func TestAgentDocumentsPromptAnnotatesEachSource(t *testing.T) {
	out := newAgentDocumentsPrompt([]workspace.AgentDocFile{
		{Path: "/a/AGENTS.md", Content: "alpha"},
		{Path: "/b/AGENTS.md", Content: "beta"},
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
		{Path: "/root/AGENTS.md", Content: strings.Repeat("a", 1000)},
		{Path: "/leaf/AGENTS.md", Content: "leaf"},
	}
	out := newAgentDocumentsPrompt(files, 200).text
	if strings.Contains(out, "/root/AGENTS.md") || !strings.Contains(out, "leaf") {
		t.Fatalf("budgeted docs = %q", out)
	}
	prompt := newAgentDocumentsPrompt(files, 200)
	if len(prompt.sources) != 1 || prompt.sources[0].Reference != "/leaf/AGENTS.md" {
		t.Fatalf("projected sources = %v", prompt.sources)
	}
	if newAgentDocumentsPrompt(nil, agentDocPromptMaxBytes).text != "" || newAgentDocumentsPrompt(files, 0).text != "" {
		t.Fatal("empty input or budget must render no prompt text")
	}
}
