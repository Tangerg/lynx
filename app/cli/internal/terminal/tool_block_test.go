package terminal

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	coreDiff "github.com/Tangerg/oolong/core/diff"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/highlight"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestPluginPresenterPanicBecomesAnError(t *testing.T) {
	_, err := presentSafely(BlockPresenter{
		Kind: agent.BlockAssistant,
		Present: func(BlockPresentation, agent.Block) []headless.Block {
			panic("present boom")
		},
	}, BlockPresentation{}, agent.Block{Kind: agent.BlockAssistant})
	if err == nil || !strings.Contains(err.Error(), "present boom") {
		t.Fatalf("presenter panic error = %v", err)
	}
}

func TestParseUnifiedDiffCarriesLineKindsAndNumbers(t *testing.T) {
	hunks := parseUnifiedDiff("--- a/a.go\n+++ b/a.go\n@@ -10,3 +10,3 @@\n keep\n-old\n+new\n")
	lines := requireSingleHunk(t, hunks, 10, 10, 3)
	if lines[0].Old != 10 || lines[0].New != 10 || lines[1].Old != 11 || lines[1].New != 0 || lines[2].Old != 0 || lines[2].New != 11 {
		t.Fatalf("numbered lines = %+v", lines)
	}
}

func requireSingleHunk(t *testing.T, hunks []coreDiff.Hunk, oldStart, newStart, lines int) []coreDiff.Line {
	t.Helper()
	if len(hunks) != 1 || hunks[0].Old != oldStart || hunks[0].New != newStart || len(hunks[0].Lines) != lines {
		t.Fatalf("hunks = %+v", hunks)
	}
	return hunks[0].Lines
}

func TestToolLabelUsesSemanticKindInsteadOfProviderName(t *testing.T) {
	call := agent.ToolCall{Kind: agent.ToolShell, Name: "opaque_provider_17", Command: "go test ./...", Summary: "ignored fallback"}
	if got := toolLabel(call); got != "$ go test ./..." || strings.Contains(got, call.Name) {
		t.Fatalf("label = %q", got)
	}
	call = agent.ToolCall{Kind: agent.ToolUnknown, Name: "custom", Summary: "do work"}
	if got := toolLabel(call); got != "custom · do work" {
		t.Fatalf("unknown label = %q", got)
	}
}

func TestToolDetailTruncationKeepsTheBeginningAndEnd(t *testing.T) {
	lines := make([]string, maxToolDetailLines+50)
	for i := range lines {
		lines[i] = "line " + string(rune(0x1000+i))
	}
	got := truncateToolDetail(strings.Join(lines, "\n"))
	if !strings.Contains(got, lines[0]) || !strings.Contains(got, lines[len(lines)-1]) || !strings.Contains(got, "70 lines omitted") {
		t.Fatalf("truncated detail did not preserve context: %q", got)
	}
}

func TestToolKindsBuildSpecializedOolongBlocks(t *testing.T) {
	presentation := BlockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}
	diff := "--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-old\n+new\n"
	tests := []struct {
		name string
		call agent.ToolCall
		want string
	}{
		{name: "shell", call: agent.ToolCall{Kind: agent.ToolShell, Status: agent.ToolOK, Output: "ok"}, want: "code"},
		{name: "read", call: agent.ToolCall{Kind: agent.ToolRead, Status: agent.ToolOK, Path: "main.go", Output: "package main"}, want: "numbered-code"},
		{name: "edit", call: agent.ToolCall{Kind: agent.ToolEdit, Status: agent.ToolOK, Path: "a.go", Diff: diff}, want: "diff"},
		{name: "search", call: agent.ToolCall{Kind: agent.ToolSearch, Status: agent.ToolOK, Query: "needle", Output: "a.go:1"}, want: "paragraph"},
		{name: "web", call: agent.ToolCall{Kind: agent.ToolWeb, Status: agent.ToolOK, URL: "https://example.com", Output: "https://example.com/result"}, want: "linked-paragraph"},
		{name: "task", call: agent.ToolCall{Kind: agent.ToolTask, Status: agent.ToolOK, Summary: "delegate", Output: "done"}, want: "paragraph"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			block := newToolBlock(presentation, agent.Block{ID: test.name, Kind: agent.BlockTool, Tool: &test.call})
			if len(block.body) == 0 {
				t.Fatal("tool built no detail body")
			}
			requireToolBody(t, block.body[0], test.want)
		})
	}
}

func TestUpdatingARunningToolPreservesItsDetailChoice(t *testing.T) {
	presentation := BlockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}
	running := agent.ToolCall{Kind: agent.ToolShell, Command: "go test ./...", Status: agent.ToolRunning}
	block := newToolBlock(presentation, agent.Block{ID: "tool", Kind: agent.BlockTool, Tool: &running})
	block.ToggleExpanded()

	completed := running
	completed.Status = agent.ToolOK
	completed.Output = "ok"
	block.Update(agent.Block{ID: "tool", Kind: agent.BlockTool, Tool: &completed})
	if !block.Expanded() {
		t.Fatal("tool completion discarded the reader's expanded state")
	}
}

func TestToolBlockStreamsOutputWithoutLosingItsDetailChoice(t *testing.T) {
	presentation := BlockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}
	running := agent.ToolCall{Kind: agent.ToolShell, Command: "go test ./...", Status: agent.ToolRunning}
	block := newToolBlock(presentation, agent.Block{ID: "tool", Kind: agent.BlockTool, Tool: &running})
	if !block.Expandable() {
		t.Fatal("running tool was not expandable before its first output")
	}
	block.SetExpanded(true)
	block.AppendOutput("first\n")
	block.AppendOutput("second\n")
	if !block.Expanded() {
		t.Fatal("streaming output discarded the expanded state")
	}
	if got := block.call.Output; got != "first\nsecond\n" {
		t.Fatalf("streamed output = %q", got)
	}
	drawn := drawToolBlock(block, 48)
	if !strings.Contains(drawn, "first") || !strings.Contains(drawn, "second") {
		t.Fatalf("streamed output was not rendered:\n%s", drawn)
	}
}

func TestCompletedToolWithoutDetailsCannotExpand(t *testing.T) {
	presentation := BlockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}
	completed := agent.ToolCall{Kind: agent.ToolShell, Command: "true", Status: agent.ToolOK}
	block := newToolBlock(presentation, agent.Block{ID: "tool", Kind: agent.BlockTool, Tool: &completed})
	if block.Expandable() || block.Expanded() {
		t.Fatal("detail-free completed tool was expandable")
	}
	block.SetExpanded(true)
	if block.ToggleExpanded() || block.Expanded() {
		t.Fatal("detail-free completed tool accepted an expansion request")
	}
	if got := block.Measure(48); got != 2 {
		t.Fatalf("detail-free tool height = %d, want header plus gap", got)
	}
	toggle, _, _, _ := block.header()
	if toggle != presentation.Glyphs.Bullet {
		t.Fatalf("detail-free tool toggle = %q, want bullet %q", toggle, presentation.Glyphs.Bullet)
	}
}

func TestToolBlockDrawsALocaleSafeStatusRailThroughExpandedDetails(t *testing.T) {
	theme, glyphs := kit.Dark(), kit.ASCII()
	call := agent.ToolCall{
		Kind: agent.ToolShell, Command: "go test ./...", Status: agent.ToolOK, Output: "all packages passed",
	}
	block := newToolBlock(BlockPresentation{Theme: theme, Glyphs: glyphs, Syntax: highlight.New("github-dark")}, agent.Block{
		ID: "test", Kind: agent.BlockTool, Tool: &call,
	})
	block.SetExpanded(true)
	width, height := 48, block.Measure(48)
	surface := grid.NewSurface(width, height)
	block.Draw(surface.View())

	drawn := strings.Join(surface.Rows(), "\n")
	for _, want := range []string{"- $ go test ./...", "x done", "all packages passed"} {
		if !strings.Contains(drawn, want) {
			t.Errorf("tool block does not contain %q:\n%s", want, drawn)
		}
	}
	for row := range height - 1 {
		cell, ok := surface.CellAt(0, row)
		if !ok || cell.Content != glyphs.Vertical || cell.Style != theme.Success {
			t.Fatalf("status rail row %d = %+v, want %q with success style", row, cell, glyphs.Vertical)
		}
	}
	if strings.Contains(drawn, "✓") || strings.Contains(drawn, "…") {
		t.Fatalf("ASCII tool block contains a Unicode-only status glyph:\n%s", drawn)
	}

	rows := block.Rows(width)
	if len(rows) != height || strings.HasPrefix(rows[0].Text, glyphs.Vertical) {
		t.Fatalf("copied rows include visual rail or have wrong height: %+v", rows)
	}
}

func TestToolStatusVocabularyDoesNotCollideWithRunOutcomes(t *testing.T) {
	presentation := BlockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode()}
	for _, test := range []struct {
		status agent.ToolStatus
		want   string
	}{
		{status: agent.ToolOK, want: "done"},
		{status: agent.ToolError, want: "error"},
		{status: agent.ToolCanceled, want: "canceled"},
		{status: agent.ToolRunning, want: "running"},
	} {
		call := agent.ToolCall{Kind: agent.ToolTask, Status: test.status}
		block := newToolBlock(presentation, agent.Block{Kind: agent.BlockTool, Tool: &call})
		_, _, status, _ := block.header()
		if !strings.Contains(status, test.want) || strings.Contains(status, "complete") || strings.Contains(status, "failed") {
			t.Errorf("tool status %q = %q", test.status, status)
		}
	}
}

func requireToolBody(t *testing.T, body headless.Block, want string) {
	t.Helper()
	switch want {
	case "code":
		requireCodeBody(t, body, false)
	case "numbered-code":
		requireCodeBody(t, body, true)
	case "diff":
		requireBodyType[*kit.Diff](t, body, "diff")
	case "paragraph":
		requireBodyType[*kit.Paragraph](t, body, "paragraph")
	case "linked-paragraph":
		requireLinkedParagraph(t, body)
	}
}

func requireCodeBody(t *testing.T, body headless.Block, numbered bool) {
	t.Helper()
	code, ok := body.(*kit.Code)
	if !ok {
		t.Fatalf("body = %T, want code", body)
	}
	if numbered && code.Gutter == nil {
		t.Fatalf("body = %#v, want numbered code", body)
	}
}

func requireBodyType[T any](t *testing.T, body headless.Block, name string) {
	t.Helper()
	if _, ok := body.(T); !ok {
		t.Fatalf("body = %T, want %s", body, name)
	}
}

func requireLinkedParagraph(t *testing.T, body headless.Block) {
	t.Helper()
	paragraph, ok := body.(*kit.Paragraph)
	if !ok {
		t.Fatalf("body = %T, want linked paragraph", body)
	}
	destination, ok := paragraph.LinkAt(0, 0, 80)
	if !ok || destination.Target != "https://example.com/result" {
		t.Fatalf("link = %#v, %v, want detected web destination", destination, ok)
	}
}

func drawToolBlock(block *toolBlock, width int) string {
	surface := grid.NewSurface(width, block.Measure(width))
	block.Draw(surface.View())
	return strings.Join(surface.Rows(), "\n")
}
