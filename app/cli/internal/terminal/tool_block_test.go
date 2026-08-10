package terminal

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	coreDiff "github.com/Tangerg/oolong/core/diff"
	"github.com/Tangerg/oolong/highlight"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

func TestPluginPresenterPanicBecomesAnError(t *testing.T) {
	_, err := presentSafely(BlockPresenter{
		Kind: client.BlockAssistant,
		Present: func(Presentation, client.Block) []headless.Block {
			panic("present boom")
		},
	}, Presentation{}, client.Block{Kind: client.BlockAssistant})
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

func requireSingleHunk(t *testing.T, hunks []coreDiff.Hunk, old, new, lines int) []coreDiff.Line {
	t.Helper()
	if len(hunks) != 1 || hunks[0].Old != old || hunks[0].New != new || len(hunks[0].Lines) != lines {
		t.Fatalf("hunks = %+v", hunks)
	}
	return hunks[0].Lines
}

func TestToolLabelUsesSemanticKindInsteadOfProviderName(t *testing.T) {
	call := client.ToolCall{Kind: client.ToolShell, Name: "opaque_provider_17", Command: "go test ./...", Summary: "ignored fallback"}
	if got := toolLabel(call); got != "$ go test ./..." || strings.Contains(got, call.Name) {
		t.Fatalf("label = %q", got)
	}
	call = client.ToolCall{Kind: client.ToolUnknown, Name: "custom", Summary: "do work"}
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
	presentation := Presentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.Style("github-dark")}
	diff := "--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-old\n+new\n"
	tests := []struct {
		name string
		call client.ToolCall
		want string
	}{
		{name: "shell", call: client.ToolCall{Kind: client.ToolShell, Status: client.ToolOK, Output: "ok"}, want: "code"},
		{name: "read", call: client.ToolCall{Kind: client.ToolRead, Status: client.ToolOK, Path: "main.go", Output: "package main"}, want: "numbered-code"},
		{name: "edit", call: client.ToolCall{Kind: client.ToolEdit, Status: client.ToolOK, Path: "a.go", Diff: diff}, want: "diff"},
		{name: "search", call: client.ToolCall{Kind: client.ToolSearch, Status: client.ToolOK, Query: "needle", Output: "a.go:1"}, want: "paragraph"},
		{name: "web", call: client.ToolCall{Kind: client.ToolWeb, Status: client.ToolOK, URL: "https://example.com", Output: "https://example.com/result"}, want: "linked-paragraph"},
		{name: "task", call: client.ToolCall{Kind: client.ToolTask, Status: client.ToolOK, Summary: "delegate", Output: "done"}, want: "paragraph"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			block := newToolBlock(presentation, client.Block{ID: test.name, Kind: client.BlockTool, Tool: &test.call})
			if len(block.body) == 0 {
				t.Fatal("tool built no detail body")
			}
			requireToolBody(t, block.body[0], test.want)
		})
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
		requireBodyType[kit.Diff](t, body, "diff")
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
	if !paragraph.Links {
		t.Fatalf("body = %#v, want links enabled", body)
	}
}
