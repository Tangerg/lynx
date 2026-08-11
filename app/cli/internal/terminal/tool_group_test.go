package terminal

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestAdjacentResourceToolsShareOneDisclosureWithoutLosingChildDetails(t *testing.T) {
	view := testTranscriptView(t)
	read := resourceTool(view, agent.ToolRead, "read · main.go", "package main")
	first := view.addGroupedTool("run-1", read)
	search := resourceTool(view, agent.ToolSearch, "search · TODO", "main.go:8")
	second := view.addGroupedTool("run-1", search)
	if first != second || view.content.Len() != 1 || len(first.block.tools) != 2 {
		t.Fatalf("adjacent group = first %p second %p blocks %d children %d", first, second, view.content.Len(), len(first.block.tools))
	}
	if view.content.Finished(first.id) {
		t.Fatal("open adjacency window was marked finished before its boundary")
	}
	view.SealToolGroups()
	if !view.content.Finished(first.id) {
		t.Fatal("settled group did not finish at its semantic boundary")
	}

	first.block.SetExpanded(true)
	surface := grid.NewSurface(72, first.block.Measure(72))
	first.block.Draw(surface.View())
	drawn := strings.Join(surface.Rows(), "\n")
	for _, expected := range []string{"2 resource operations", "read · main.go", "package main", "search · TODO", "main.go:8"} {
		if !strings.Contains(drawn, expected) {
			t.Fatalf("expanded tool group does not contain %q:\n%s", expected, drawn)
		}
	}
	document := first.block.readerDocument()
	if document.Title != "2 resource operations" || len(document.Sections) < 4 {
		t.Fatalf("group reader document = %+v", document)
	}
}

func TestAConversationBlockClosesToolAdjacency(t *testing.T) {
	view := testTranscriptView(t)
	first := view.addGroupedTool("run-1", resourceTool(view, agent.ToolRead, "read · a.go", "a"))
	view.Append(newUserMessageBlock(view.theme, "semantic boundary"))
	second := view.addGroupedTool("run-1", resourceTool(view, agent.ToolWeb, "web · docs", "docs"))
	if first == second || view.content.Len() != 3 {
		t.Fatalf("boundary did not split groups: first %p second %p blocks %d", first, second, view.content.Len())
	}
	if !view.content.Finished(first.id) {
		t.Fatal("first group remained unfinished after a conversation boundary")
	}
}

func TestLiveGroupedToolFinishesOnlyAfterItsAdjacencyWindowCloses(t *testing.T) {
	view := testTranscriptView(t)
	call := agent.ToolCall{Kind: agent.ToolRead, Path: "live.go", Status: agent.ToolRunning}
	tool := newToolBlock(toolGroupPresentation(view), agent.Block{ID: "read", RunID: "run-1", Kind: agent.BlockTool, Tool: &call})
	group := view.addGroupedTool("run-1", tool)
	tracked := trackedTool{id: group.id, block: tool}
	view.tools["read"] = liveTool{blocks: []trackedTool{tracked}, group: group}
	if err := view.deltaTool("read", "package live\n"); err != nil {
		t.Fatal(err)
	}
	completed := call
	completed.Status = agent.ToolOK
	completed.Output = "package final\n"
	if !view.completeLiveTool(agent.Block{ID: "read", RunID: "run-1", Kind: agent.BlockTool, Tool: &completed}) {
		t.Fatal("live grouped tool was not completed")
	}
	if view.content.Finished(group.id) {
		t.Fatal("completed child prematurely closed the adjacency group")
	}
	if got := group.block.readerDocument(); !readerDocumentContains(got, "package final") || readerDocumentContains(got, "package live") {
		t.Fatalf("reader did not receive authoritative grouped-tool content: %+v", got)
	}
	view.SealToolGroups()
	if !view.content.Finished(group.id) {
		t.Fatal("closed grouped tool did not become retainable")
	}
}

func resourceTool(view *transcriptView, kind agent.ToolKind, summary, output string) *toolBlock {
	call := agent.ToolCall{Kind: kind, Summary: summary, Output: output, Status: agent.ToolOK}
	switch kind {
	case agent.ToolRead:
		call.Path = strings.TrimPrefix(summary, "read · ")
	case agent.ToolSearch:
		call.Query = strings.TrimPrefix(summary, "search · ")
	case agent.ToolWeb:
		call.URL = strings.TrimPrefix(summary, "web · ")
	}
	return newToolBlock(toolGroupPresentation(view), agent.Block{Kind: agent.BlockTool, Tool: &call})
}

func toolGroupPresentation(view *transcriptView) BlockPresentation {
	return BlockPresentation{
		Theme: view.theme, Glyphs: view.glyphs, Look: view.look,
		Syntax: view.syntax, Tools: defaultToolPresenters(),
	}
}

func readerDocumentContains(document readerDocument, expected string) bool {
	for _, section := range document.Sections {
		if strings.Contains(section.Text, expected) {
			return true
		}
	}
	return false
}
