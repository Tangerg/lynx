package sessionexport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestParseFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Format
		wantErr bool
	}{
		{name: "markdown", input: " markdown ", want: Markdown},
		{name: "markdown shorthand", input: "MD", want: Markdown},
		{name: "json", input: "JSON", want: JSON},
		{name: "unknown", input: "yaml", wantErr: true},
		{name: "empty", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseFormat(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("ParseFormat(%q) unexpectedly succeeded with %q", test.input, got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("ParseFormat(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestSessionExportRendersStableMarkdownAndJSON(t *testing.T) {
	snapshot := exportFixture()
	snapshot.Session.Workspace = "/tmp/work`space"
	markdown, err := New(snapshot, Markdown)
	if err != nil {
		t.Fatal(err)
	}
	markdownBytes, err := markdown.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	markdownText := string(markdownBytes)
	for _, expected := range []string{"# Export fixture", "- Workspace: ``/tmp/work`space``", "### You", "### Lyra", "### Tool · go test ./...", "````text", "## Plan"} {
		if !strings.Contains(markdownText, expected) {
			t.Fatalf("markdown does not contain %q:\n%s", expected, markdownText)
		}
	}

	jsonExport, err := New(snapshot, JSON)
	if err != nil {
		t.Fatal(err)
	}
	jsonBytes, err := jsonExport.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(jsonBytes)
	for _, expected := range []string{`"schemaVersion": 1`, `"sessionId": "session-1"`, `"durationNanoseconds": 2000000000`, `"kind": "shell"`} {
		if !strings.Contains(jsonText, expected) {
			t.Fatalf("json does not contain %q:\n%s", expected, jsonText)
		}
	}
	if strings.Contains(jsonText, `"ID"`) || strings.Contains(jsonText, `"Status"`) {
		t.Fatalf("json leaked Go field naming:\n%s", jsonText)
	}
}

func TestSessionExportPublishesWithoutEscapingOrOverwriting(t *testing.T) {
	workspace := t.TempDir()
	report, err := New(exportFixture(), Markdown)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := report.Save(workspace, "../escape.md"); err == nil {
		t.Fatal("path traversal export name was accepted")
	}
	if _, err := report.Save(workspace, `..\escape.md`); err == nil {
		t.Fatal("Windows path traversal export name was accepted")
	}
	if _, err := report.Save("", "report.md"); err == nil {
		t.Fatal("empty export workspace was accepted")
	}
	first, err := report.Save(workspace, "report?.md")
	if err != nil {
		t.Fatal(err)
	}
	resolvedWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(first) != resolvedWorkspace {
		t.Fatalf("export escaped workspace: %s", first)
	}
	info, err := os.Stat(first)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("export mode = %o, want 600", info.Mode().Perm())
	}
	identical, err := report.Save(workspace, "report?.md")
	if err != nil {
		t.Fatal(err)
	}
	if identical != first {
		t.Fatalf("identical export = %s, want deduplicated %s", identical, first)
	}

	changed := exportFixture()
	changed.Transcript[1].Text = "A different answer."
	secondReport, err := New(changed, Markdown)
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondReport.Save(workspace, "report?.md")
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("different export overwrote the existing file")
	}
}

func TestLastAssistantTextUsesTheLatestDurableAssistantBlock(t *testing.T) {
	snapshot := exportFixture()
	snapshot.Transcript = append(snapshot.Transcript, agent.Block{
		ID: "assistant-2", RunID: "run-1", Status: agent.BlockStatusCompleted,
		Kind: agent.BlockAssistant, Text: "  latest answer  ",
	})
	text, err := LastAssistantText(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if text != "latest answer" {
		t.Fatalf("last assistant text = %q", text)
	}
}

func exportFixture() agent.SessionSnapshot {
	exitCode := 0
	return agent.SessionSnapshot{
		Session: agent.Session{
			ID: "session-1", Title: "Export fixture", Status: agent.SessionIdle,
			Model: "mock/model", Workspace: "/tmp/export-fixture",
			CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(2, 0).UTC(), Revision: 3,
		},
		Transcript: []agent.Block{
			{ID: "user-1", RunID: "run-1", Status: agent.BlockStatusCompleted, Kind: agent.BlockUser, Text: "Run the tests."},
			{ID: "assistant-1", RunID: "run-1", Status: agent.BlockStatusCompleted, Kind: agent.BlockAssistant, Text: "Tests passed."},
			{ID: "tool-1", RunID: "run-1", Status: agent.BlockStatusCompleted, Kind: agent.BlockTool, Tool: &agent.ToolCall{
				Kind: agent.ToolShell, Name: "shell", Summary: "go test ./...", Status: agent.ToolOK,
				Command: "go test ./...", Output: "ok\n```\n", ExitCode: &exitCode, Duration: 2 * time.Second,
			}},
		},
		Runs: []agent.Run{{
			ID: "run-1", SessionID: "session-1", Provider: "mock", Model: "model",
			Status: agent.RunStatusFinished, Outcome: agent.Outcome{Status: agent.OutcomeCompleted},
			Usage: agent.Usage{InputTokens: 10, OutputTokens: 20, Duration: 2 * time.Second},
		}},
		PlanRevision: 1,
		Plan:         []agent.PlanItem{{Title: "Run tests", Status: agent.PlanDone}},
	}
}
