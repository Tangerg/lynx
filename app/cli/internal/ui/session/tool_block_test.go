package session

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
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
	if len(hunks) != 1 || hunks[0].Old != 10 || hunks[0].New != 10 || len(hunks[0].Lines) != 3 {
		t.Fatalf("hunks = %+v", hunks)
	}
	lines := hunks[0].Lines
	if lines[0].Old != 10 || lines[0].New != 10 || lines[1].Old != 11 || lines[1].New != 0 || lines[2].Old != 0 || lines[2].New != 11 {
		t.Fatalf("numbered lines = %+v", lines)
	}
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
		name  string
		call  client.ToolCall
		check func(t *testing.T, block *toolBlock)
	}{
		{name: "shell", call: client.ToolCall{Kind: client.ToolShell, Status: client.ToolOK, Output: "ok"}, check: func(t *testing.T, block *toolBlock) {
			if _, ok := block.body[0].(*kit.Code); !ok {
				t.Fatalf("shell body = %T", block.body[0])
			}
		}},
		{name: "read", call: client.ToolCall{Kind: client.ToolRead, Status: client.ToolOK, Path: "main.go", Output: "package main"}, check: func(t *testing.T, block *toolBlock) {
			code, ok := block.body[0].(*kit.Code)
			if !ok || code.Gutter == nil {
				t.Fatalf("read body = %#v", block.body[0])
			}
		}},
		{name: "edit", call: client.ToolCall{Kind: client.ToolEdit, Status: client.ToolOK, Path: "a.go", Diff: diff}, check: func(t *testing.T, block *toolBlock) {
			if _, ok := block.body[0].(kit.Diff); !ok {
				t.Fatalf("edit body = %T", block.body[0])
			}
		}},
		{name: "search", call: client.ToolCall{Kind: client.ToolSearch, Status: client.ToolOK, Query: "needle", Output: "a.go:1"}, check: func(t *testing.T, block *toolBlock) {
			if _, ok := block.body[0].(*kit.Paragraph); !ok {
				t.Fatalf("search body = %T", block.body[0])
			}
		}},
		{name: "web", call: client.ToolCall{Kind: client.ToolWeb, Status: client.ToolOK, URL: "https://example.com", Output: "https://example.com/result"}, check: func(t *testing.T, block *toolBlock) {
			paragraph, ok := block.body[0].(*kit.Paragraph)
			if !ok || !paragraph.Links {
				t.Fatalf("web body = %#v", block.body[0])
			}
		}},
		{name: "task", call: client.ToolCall{Kind: client.ToolTask, Status: client.ToolOK, Summary: "delegate", Output: "done"}, check: func(t *testing.T, block *toolBlock) {
			if _, ok := block.body[0].(*kit.Paragraph); !ok {
				t.Fatalf("task body = %T", block.body[0])
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			block := newToolBlock(presentation, client.Block{ID: test.name, Kind: client.BlockTool, Tool: &test.call})
			if len(block.body) == 0 {
				t.Fatal("tool built no detail body")
			}
			test.check(t, block)
		})
	}
}
