package terminal

import (
	"image"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/highlight"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

func TestStreamingPreservesAReadersScrollPosition(t *testing.T) {
	view := testConversationView(t)
	root := headless.NewRoot(view)
	surface := grid.NewSurface(32, 5)
	started := client.Block{ID: "answer", Kind: client.BlockAssistant}
	if err := view.Apply(client.BlockStarted{Block: started}, nil); err != nil {
		t.Fatal(err)
	}
	initial := strings.Repeat("a paragraph long enough to occupy rows\n\n", 12)
	if err := view.Apply(client.BlockDelta{BlockID: started.ID, Text: initial}, nil); err != nil {
		t.Fatal(err)
	}
	root.Draw(surface.View())
	if !view.scroll.AtBottom() {
		t.Fatal("a new transcript did not follow its output")
	}
	if !view.Scroll(scrollPageUp) || view.scroll.AtBottom() {
		t.Fatal("page-up did not enter reader-controlled scrolling")
	}
	wantOffset := view.scroll.Offset()

	if err := view.Apply(client.BlockDelta{BlockID: started.ID, Text: "new streamed tail\n\n"}, nil); err != nil {
		t.Fatal(err)
	}
	root.Draw(surface.View())
	if view.scroll.AtBottom() {
		t.Fatal("streaming resumed bottom-following after the reader scrolled up")
	}
	if got := view.scroll.Offset(); got != wantOffset {
		t.Fatalf("streaming moved reader from offset %d to %d", wantOffset, got)
	}
}

func TestClickingAToolHeaderTogglesOnlyThatTool(t *testing.T) {
	view := testConversationView(t)
	first := appendTestTool(view, "first", "FIRST_DETAIL")
	second := appendTestTool(view, "second", "SECOND_DETAIL")
	root := headless.NewRoot(view)
	surface := grid.NewSurface(48, 8)
	root.Draw(surface.View())

	if !root.Handle(input.Mouse{Pos: image.Pt(0, 0), Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("tool header click was not handled")
	}
	if first.expanded {
		t.Fatal("tool expanded before the pointer gesture committed")
	}
	if !root.Handle(input.Mouse{Pos: image.Pt(0, 0), Action: input.MouseUp, Button: input.ButtonLeft}) {
		t.Fatal("tool header release was not handled")
	}
	if !first.expanded {
		t.Fatal("committed tool click did not expand")
	}
	if second.expanded {
		t.Fatal("clicking one tool expanded a different tool")
	}
	if view.selection.Active() {
		t.Fatal("activating a tool left a text selection behind")
	}

	root.Draw(surface.View())
	if !root.Handle(input.Mouse{Pos: image.Pt(0, 0), Action: input.MouseDown, Button: input.ButtonLeft}) ||
		!root.Handle(input.Mouse{Pos: image.Pt(0, 0), Action: input.MouseUp, Button: input.ButtonLeft}) {
		t.Fatal("expanded tool header click was not handled")
	}
	if first.expanded {
		t.Fatal("second click did not collapse the tool")
	}
}

func TestDraggingFromAToolHeaderCopiesWithoutToggling(t *testing.T) {
	clipboard := new(recordingClipboard)
	view := newConversationView(kit.Dark(), kit.Unicode(), input.Wheel{}, highlight.Style("github-dark"), 24, false, clipboard)
	t.Cleanup(view.Close)
	tool := appendTestTool(view, "drag", "DRAG_DETAIL")
	root := headless.NewRoot(view)
	surface := grid.NewSurface(48, 8)
	root.Draw(surface.View())

	if !root.Handle(input.Mouse{Pos: image.Pt(4, 0), Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("selection press was not handled")
	}
	if !view.selection.Dragging() {
		t.Fatal("entry selection cleared the transcript drag owner")
	}
	if !root.Handle(input.Mouse{Pos: image.Pt(9, 0), Action: input.MouseDrag, Button: input.ButtonLeft}) ||
		!root.Handle(input.Mouse{Pos: image.Pt(9, 0), Action: input.MouseUp, Button: input.ButtonLeft}) {
		t.Fatal("selection drag was not handled")
	}
	if clipboard.text == "" {
		t.Fatal("drag selection was not copied")
	}
	if tool.expanded {
		t.Fatal("drag selection toggled the tool layout")
	}
}

func TestEscapeClearsATranscriptTextSelectionBeforeOtherActions(t *testing.T) {
	view := testConversationView(t)
	appendTestTool(view, "selection", "SELECTION_DETAIL")
	view.Focus(true)
	root := headless.NewRoot(view)
	surface := grid.NewSurface(48, 8)
	root.Draw(surface.View())
	root.Handle(input.Mouse{Pos: image.Pt(4, 0), Action: input.MouseDown, Button: input.ButtonLeft})
	root.Handle(input.Mouse{Pos: image.Pt(9, 0), Action: input.MouseDrag, Button: input.ButtonLeft})
	root.Handle(input.Mouse{Pos: image.Pt(9, 0), Action: input.MouseUp, Button: input.ButtonLeft})
	if !view.selection.Active() {
		t.Fatal("test did not create a transcript selection")
	}
	if !view.Handle(input.Key{Code: input.Esc}) || view.selection.Active() {
		t.Fatal("Esc did not clear the transcript selection first")
	}
}

func TestGlobalToolToggleNormalizesMixedDetailStates(t *testing.T) {
	view := testConversationView(t)
	first := appendTestTool(view, "first", "FIRST_DETAIL")
	second := appendTestTool(view, "second", "SECOND_DETAIL")
	first.ToggleExpanded()

	view.ToggleDetails()
	if !first.Expanded() || !second.Expanded() {
		t.Fatal("expand-all did not normalize mixed tool states")
	}
	view.ToggleDetails()
	if first.Expanded() || second.Expanded() {
		t.Fatal("collapse-all left an expanded tool")
	}
}

func TestCompletingALiveToolPreservesItsExpandedState(t *testing.T) {
	view := testConversationView(t)
	tool := appendTestTool(view, "tool", "running")
	tracked := view.toolViews[0]
	view.tools["tool"] = liveTool{ids: []headless.BlockID{tracked.id}, blocks: []trackedTool{tracked}}
	tool.ToggleExpanded()

	completed := client.ToolCall{Kind: client.ToolShell, Command: "echo tool", Output: "complete", Status: client.ToolOK}
	if !view.completeLiveTool(client.Block{ID: "tool", Kind: client.BlockTool, Tool: &completed}) {
		t.Fatal("live tool was not completed in place")
	}
	if !tool.Expanded() {
		t.Fatal("live tool completion discarded the reader's expanded state")
	}
}

func TestTranscriptFocusSelectsAndOperatesOnOneEntry(t *testing.T) {
	view := testConversationView(t)
	first := appendTestTool(view, "first", "FIRST_DETAIL")
	second := appendTestTool(view, "second", "SECOND_DETAIL")

	view.Focus(true)
	if !view.hasSelected || view.selected != view.toolViews[1].id {
		t.Fatalf("initial selection = %d, want newest entry %d", view.selected, view.toolViews[1].id)
	}
	if !view.Handle(input.Key{Code: input.Up}) {
		t.Fatal("focused transcript did not handle Up")
	}
	if view.selected != view.toolViews[0].id {
		t.Fatalf("Up selected %d, want %d", view.selected, view.toolViews[0].id)
	}
	if !view.Handle(input.Key{Code: input.Right}) || !first.expanded {
		t.Fatal("Right did not expand the selected tool")
	}
	if second.expanded {
		t.Fatal("expanding the selected tool changed another entry")
	}
	if !view.Handle(input.Key{Code: input.Enter}) || first.expanded {
		t.Fatal("Enter did not toggle the selected tool")
	}

	view.Focus(false)
	if view.entries[view.selected].focused {
		t.Fatal("selected entry retained the focused rail after transcript blur")
	}
}

func TestFocusedTranscriptCopiesTheSelectedBlock(t *testing.T) {
	clipboard := new(recordingClipboard)
	view := newConversationView(kit.Dark(), kit.Unicode(), input.Wheel{}, highlight.Style("github-dark"), 24, false, clipboard)
	t.Cleanup(view.Close)
	appendTestTool(view, "copy", "COPY_DETAIL")
	view.Focus(true)
	headless.NewRoot(view).Draw(grid.NewSurface(48, 8).View())

	if !view.Handle(input.Key{Code: input.Character, Rune: 'c', Mods: input.Alt}) {
		t.Fatal("Alt+C was not handled by the focused transcript")
	}
	if !strings.Contains(clipboard.text, "echo copy") {
		t.Fatalf("clipboard = %q, want selected tool content", clipboard.text)
	}
}

type recordingClipboard struct{ text string }

func (c *recordingClipboard) Copy(value string) bool {
	c.text = value
	return true
}

func (*recordingClipboard) Paste() bool { return false }

func testConversationView(t *testing.T) *conversationView {
	t.Helper()
	view := newConversationView(kit.Dark(), kit.Unicode(), input.Wheel{}, highlight.Style("github-dark"), 24, false, nil)
	t.Cleanup(view.Close)
	return view
}

func appendTestTool(view *conversationView, id, output string) *toolBlock {
	call := client.ToolCall{Kind: client.ToolShell, Command: "echo " + id, Output: output, Status: client.ToolOK}
	block := newToolBlock(Presentation{Theme: view.theme, Glyphs: view.glyphs, Look: view.look, Syntax: view.syntax}, client.Block{
		ID: id, Kind: client.BlockTool, Tool: &call,
	})
	blockID := view.place(block, true)
	view.toolViews = append(view.toolViews, trackedTool{id: blockID, block: block})
	return block
}
