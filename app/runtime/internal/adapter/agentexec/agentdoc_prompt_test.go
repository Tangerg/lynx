package agentexec

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
)

func TestRenderAgentDocsAnnotatesEachSource(t *testing.T) {
	out := renderAgentDocs([]workspace.AgentDocFile{
		{Path: "/a/AGENTS.md", Content: "alpha"},
		{Path: "/b/AGENTS.md", Content: "beta"},
	}, agentDocPromptMaxBytes)

	for _, want := range []string{"<!-- From: /a/AGENTS.md -->", "<!-- From: /b/AGENTS.md -->", "alpha", "beta"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered docs = %q, missing %q", out, want)
		}
	}
	if strings.Index(out, "alpha") > strings.Index(out, "beta") {
		t.Fatalf("docs are out of order: %q", out)
	}
}

func TestRenderAgentDocsKeepsMostSpecificFilesWithinBudget(t *testing.T) {
	files := []workspace.AgentDocFile{
		{Path: "/root/AGENTS.md", Content: strings.Repeat("a", 1000)},
		{Path: "/leaf/AGENTS.md", Content: "leaf"},
	}
	out := renderAgentDocs(files, 200)
	if strings.Contains(out, "/root/AGENTS.md") || !strings.Contains(out, "leaf") {
		t.Fatalf("budgeted docs = %q", out)
	}
	if renderAgentDocs(nil, agentDocPromptMaxBytes) != "" || renderAgentDocs(files, 0) != "" {
		t.Fatal("empty input or budget must render no prompt text")
	}
}
