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

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
)

func TestCustomRuntimeEventsUseNamedTerminalPresenters(t *testing.T) {
	view := testTranscriptView(t)
	registry := new(extensions.Registry)
	loaded, err := extensions.Load(registry, extensions.Plugin{
		ID: "test.custom-events", Version: "1.0.0", APIVersion: extensions.HostAPIVersion,
		Capabilities: []extensions.Capability{CustomEventPresenters.Capability()},
		Setup: func(scope *extensions.Scope) error {
			_, err := extensions.Contribute(scope, CustomEventPresenters, CustomEventPresenter{
				Name: "vendor.trace",
				Present: func(presentation BlockPresentation, event agent.CustomEvent) []headless.Block {
					return []headless.Block{&kit.Message{Theme: presentation.Theme, Speaker: "trace", Body: string(event.PayloadJSON)}}
				},
			}, extensions.Contribution{})
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loaded.Dispose() })

	if err := view.ApplyRunEvent(agent.RunEvent{
		RunID: "run_1", Event: agent.CustomEvent{Name: "vendor.trace", PayloadJSON: []byte(`{"span":"abc"}`)},
	}, registry); err != nil {
		t.Fatal(err)
	}
	if err := view.ApplyRunEvent(agent.RunEvent{
		RunID: "run_1", Event: agent.CustomEvent{Name: "vendor.unhandled", PayloadJSON: []byte(`null`)},
	}, registry); err != nil {
		t.Fatal(err)
	}
	if len(view.runEntries["run_1"]) != 1 {
		t.Fatalf("custom event entries = %+v", view.runEntries)
	}
	drawn := drawRoot(t, view, 48, 6)
	if !strings.Contains(drawn, "trace") || !strings.Contains(drawn, `"span":"abc"`) {
		t.Fatalf("custom event rendering =\n%s", drawn)
	}
}

func TestStreamingPreservesAReadersScrollPosition(t *testing.T) {
	view := testTranscriptView(t)
	root := headless.NewRoot(view)
	surface := grid.NewSurface(32, 5)
	started := agent.Block{ID: "answer", Kind: agent.BlockAssistant}
	if err := view.Apply(agent.BlockStarted{Block: started}, nil); err != nil {
		t.Fatal(err)
	}
	initial := strings.Repeat("a paragraph long enough to occupy rows\n\n", 12)
	if err := view.Apply(agent.BlockDelta{BlockID: started.ID, Text: initial}, nil); err != nil {
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

	if err := view.Apply(agent.BlockDelta{BlockID: started.ID, Text: "new streamed tail\n\n"}, nil); err != nil {
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

func TestInterleavedTextBlocksStreamIndependently(t *testing.T) {
	view := testTranscriptView(t)
	for _, event := range []agent.Event{
		agent.BlockStarted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant}},
		agent.BlockDelta{BlockID: "answer", Text: "assistant provisional"},
		agent.BlockStarted{Block: agent.Block{ID: "reasoning", Kind: agent.BlockReasoning}},
		agent.BlockDelta{BlockID: "reasoning", Text: "reasoning provisional"},
		agent.BlockCompleted{Block: agent.Block{ID: "reasoning", Kind: agent.BlockReasoning, Text: "reasoning final"}},
		agent.BlockDelta{BlockID: "answer", Text: " tail"},
		agent.BlockCompleted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant, Text: "assistant final"}},
	} {
		if err := view.Apply(event, nil); err != nil {
			t.Fatalf("apply %T: %v", event, err)
		}
	}
	if len(view.textStreams) != 0 {
		t.Fatalf("completed text streams = %+v", view.textStreams)
	}
	surface := grid.NewSurface(48, 12)
	headless.NewRoot(view).Draw(surface.View())
	drawn := strings.Join(surface.Rows(), "\n")
	for _, want := range []string{"assistant final", "reasoning final"} {
		if !strings.Contains(drawn, want) {
			t.Fatalf("interleaved transcript does not contain %q:\n%s", want, drawn)
		}
	}
	if strings.Contains(drawn, "provisional") {
		t.Fatalf("authoritative completion left provisional text:\n%s", drawn)
	}
}

func TestStreamedAssistantCompletionAppendsInlineImages(t *testing.T) {
	view := testTranscriptView(t)
	started := agent.Block{ID: "answer", RunID: "run_1", Kind: agent.BlockAssistant, Status: agent.BlockStatusRunning}
	if err := view.ApplyRunEvent(agent.RunEvent{RunID: "run_1", Event: agent.BlockStarted{Block: started}}, nil); err != nil {
		t.Fatal(err)
	}
	completed := started
	completed.Status = agent.BlockStatusCompleted
	completed.Text = "Generated chart"
	completed.Images = []agent.InlineImage{{
		ID: "answer:image:0", Name: "chart.png", MIMEType: "image/png", Data: []byte("png"),
	}}
	if err := view.ApplyRunEvent(agent.RunEvent{RunID: "run_1", Event: agent.BlockCompleted{Block: completed}}, nil); err != nil {
		t.Fatal(err)
	}
	if view.content.Len() != 2 || len(view.runEntries["run_1"]) != 2 {
		t.Fatalf("streamed image entries = content %d, run entries %+v", view.content.Len(), view.runEntries)
	}
	drawn := drawRoot(t, view, 48, 8)
	if !strings.Contains(drawn, "Generated chart") || !strings.Contains(drawn, "chart.png") {
		t.Fatalf("streamed image transcript =\n%s", drawn)
	}
}

func TestToolStreamingPreservesAReadersScrollPosition(t *testing.T) {
	view := testTranscriptView(t)
	root := headless.NewRoot(view)
	surface := grid.NewSurface(40, 5)
	running := agent.ToolCall{Kind: agent.ToolShell, Command: "long command", Status: agent.ToolRunning}
	tool := beginTestTool(view, running)
	tool.SetExpanded(true)
	initial := strings.Repeat("tool output long enough to occupy rows\n", 20)
	if err := view.Apply(agent.BlockDelta{BlockID: "tool", Text: initial}, nil); err != nil {
		t.Fatal(err)
	}
	root.Draw(surface.View())
	if !view.scroll.AtBottom() {
		t.Fatal("new tool stream did not follow its output")
	}
	if !view.Scroll(scrollPageUp) || view.scroll.AtBottom() {
		t.Fatal("page-up did not enter reader-controlled scrolling")
	}
	wantOffset := view.scroll.Offset()

	if err := view.Apply(agent.BlockDelta{BlockID: "tool", Text: "new tool tail\n"}, nil); err != nil {
		t.Fatal(err)
	}
	root.Draw(surface.View())
	if view.scroll.AtBottom() {
		t.Fatal("tool streaming resumed bottom-following after the reader scrolled up")
	}
	if got := view.scroll.Offset(); got != wantOffset {
		t.Fatalf("tool streaming moved reader from offset %d to %d", wantOffset, got)
	}
}

func TestLiveToolStreamsInPlaceAndCompletesFromAuthoritativeOutput(t *testing.T) {
	view := testTranscriptView(t)
	running := agent.ToolCall{
		Kind: agent.ToolShell, Command: "go test ./...", Status: agent.ToolRunning,
		ArgumentsJSON: []byte(`{"phase":"provisional-arguments"}`),
	}
	tool := beginTestTool(view, running)
	tool.SetExpanded(true)
	for _, chunk := range []string{"first\n", "second\n"} {
		if err := view.Apply(agent.BlockDelta{BlockID: "tool", Text: chunk}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := copyableRowsText(tool.Rows(48)); !strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Fatalf("live tool rows = %q", got)
	}
	completed := agent.ToolCall{
		Kind: agent.ToolShell, Command: "go test ./...", Status: agent.ToolOK, Output: "final\n",
		ArgumentsJSON: []byte(`{"phase":"authoritative-arguments"}`),
		ResultJSON:    []byte(`{"resultMarker":"authoritative-result"}`),
	}
	if err := view.Apply(agent.BlockCompleted{Block: agent.Block{ID: "tool", Kind: agent.BlockTool, Tool: &completed}}, nil); err != nil {
		t.Fatal(err)
	}
	if !tool.Expanded() {
		t.Fatal("tool completion discarded the expanded state")
	}
	got := copyableRowsText(tool.Rows(48))
	if !strings.Contains(got, "final") || !strings.Contains(got, "authoritative-arguments") ||
		!strings.Contains(got, "authoritative-result") || strings.Contains(got, "provisional-arguments") ||
		strings.Contains(got, "first") || strings.Contains(got, "second") {
		t.Fatalf("completed tool did not use authoritative output: %q", got)
	}
}

func TestDetailFreeCompletedToolIsNotAnnouncedExpandable(t *testing.T) {
	view := testTranscriptView(t)
	var selection transcriptSelection
	view.OnSelection(func(next transcriptSelection) { selection = next })
	call := agent.ToolCall{Kind: agent.ToolShell, Command: "true", Status: agent.ToolOK}
	block := newToolBlock(BlockPresentation{Theme: view.theme, Glyphs: view.glyphs, Look: view.look, Syntax: view.syntax}, agent.Block{
		ID: "tool", Kind: agent.BlockTool, Tool: &call,
	})
	id := view.place(block, true)
	view.toolViews = append(view.toolViews, trackedToolView{id: id, block: block})
	view.Focus(true)
	if !selection.Present || selection.Expandable || selection.Expanded {
		t.Fatalf("selection announcement = %+v", selection)
	}
	if !view.Handle(input.Key{Code: input.Enter}) || block.Expanded() {
		t.Fatal("detail-free tool expanded through keyboard interaction")
	}
}

func TestCompletingASelectedToolWithoutDetailsRemovesItsExpansionAction(t *testing.T) {
	view := testTranscriptView(t)
	var selection transcriptSelection
	view.OnSelection(func(next transcriptSelection) { selection = next })
	running := agent.ToolCall{Kind: agent.ToolShell, Command: "true", Status: agent.ToolRunning}
	tool := beginTestTool(view, running)
	view.Focus(true)
	tool.SetExpanded(true)
	view.announceSelection()
	if !selection.Expandable || !selection.Expanded {
		t.Fatalf("running selection = %+v", selection)
	}
	completed := agent.ToolCall{Kind: agent.ToolShell, Command: "true", Status: agent.ToolOK}
	if err := view.Apply(agent.BlockCompleted{Block: agent.Block{ID: "tool", Kind: agent.BlockTool, Tool: &completed}}, nil); err != nil {
		t.Fatal(err)
	}
	if selection.Expandable || selection.Expanded || tool.Expanded() {
		t.Fatalf("completed selection = %+v, tool expanded = %t", selection, tool.Expanded())
	}
}

func TestCanceledRunSettlesEveryLiveTranscriptBlock(t *testing.T) {
	view := testTranscriptView(t)
	if err := view.Apply(agent.BlockStarted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := view.Apply(agent.BlockDelta{BlockID: "answer", Text: "partial answer"}, nil); err != nil {
		t.Fatal(err)
	}
	tool := beginTestTool(view, agent.ToolCall{Kind: agent.ToolShell, Command: "long command", Status: agent.ToolRunning})
	tool.SetExpanded(true)
	if err := view.Apply(agent.BlockDelta{BlockID: "tool", Text: "partial tool output\n"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := view.Apply(agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCanceled}}, nil); err != nil {
		t.Fatal(err)
	}
	if len(view.textStreams) != 0 || len(view.tools) != 0 {
		t.Fatalf("live projections survived cancellation: text=%d tools=%d", len(view.textStreams), len(view.tools))
	}
	if tool.call.Status != agent.ToolCanceled {
		t.Fatalf("settled tool status = %q", tool.call.Status)
	}
	for index := range view.content.Len() {
		id := view.content.FirstBlock() + headless.BlockID(index)
		if !view.content.Finished(id) {
			t.Fatalf("transcript block %d remained unfinished", id)
		}
	}
	surface := grid.NewSurface(48, 12)
	headless.NewRoot(view).Draw(surface.View())
	drawn := strings.Join(surface.Rows(), "\n")
	for _, want := range []string{"partial answer", "partial tool output", "canceled"} {
		if !strings.Contains(drawn, want) {
			t.Fatalf("settled transcript does not contain %q:\n%s", want, drawn)
		}
	}
}

func TestChildCompletionSettlesOnlyThatRunsCollidingBlockIdentity(t *testing.T) {
	view := testTranscriptView(t)
	rootID, childID, blockID := "run_root", "run_child", "answer"
	apply := func(runID string, event agent.Event) {
		t.Helper()
		if err := view.ApplyRunEvent(agent.RunEvent{RunID: runID, Event: event}, nil); err != nil {
			t.Fatalf("apply %T for %s: %v", event, runID, err)
		}
	}
	started := func(runID string) agent.BlockStarted {
		return agent.BlockStarted{Block: agent.Block{ID: blockID, RunID: runID, Kind: agent.BlockAssistant, Status: agent.BlockStatusRunning}}
	}
	apply(rootID, agent.SegmentStarted{Run: agent.Run{ID: rootID}})
	apply(childID, agent.SegmentStarted{Run: agent.Run{
		ID: childID, Lineage: agent.RunLineage{SpawnedByBlockID: "spawn", ParentRunID: rootID, RootRunID: rootID},
	}})
	apply(rootID, started(rootID))
	apply(rootID, agent.BlockDelta{BlockID: blockID, Text: "root partial"})
	apply(childID, started(childID))
	apply(childID, agent.BlockDelta{BlockID: blockID, Text: "child partial"})
	apply(childID, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}})

	if _, live := view.textStreams[transcriptBlockKey(childID, blockID)]; live {
		t.Fatal("child text stream survived child completion")
	}
	if _, live := view.textStreams[transcriptBlockKey(rootID, blockID)]; !live {
		t.Fatal("child completion settled the root text stream")
	}
	apply(rootID, agent.BlockDelta{BlockID: blockID, Text: " continued"})
	apply(rootID, agent.BlockCompleted{Block: agent.Block{
		ID: blockID, RunID: rootID, Kind: agent.BlockAssistant,
		Status: agent.BlockStatusCompleted, Text: "root final",
	}})

	surface := grid.NewSurface(48, 12)
	headless.NewRoot(view).Draw(surface.View())
	drawn := strings.Join(surface.Rows(), "\n")
	for _, want := range []string{"subagent · run_child", "child partial", "root final"} {
		if !strings.Contains(drawn, want) {
			t.Fatalf("tree transcript does not contain %q:\n%s", want, drawn)
		}
	}
}

func TestClickingAToolHeaderTogglesOnlyThatTool(t *testing.T) {
	view := testTranscriptView(t)
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
	view := newTranscriptView(kit.Dark(), kit.Unicode(), input.Wheel{}, highlight.New("github-dark"), 24, false, clipboard)
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
	view := testTranscriptView(t)
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
	view := testTranscriptView(t)
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
	view := testTranscriptView(t)
	tool := appendTestTool(view, "tool", "running")
	tracked := trackedTool{id: view.toolViews[0].id, block: tool}
	view.tools["tool"] = liveTool{ids: []headless.BlockID{tracked.id}, blocks: []trackedTool{tracked}}
	tool.ToggleExpanded()

	completed := agent.ToolCall{Kind: agent.ToolShell, Command: "echo tool", Output: "complete", Status: agent.ToolOK}
	if !view.completeLiveTool(agent.Block{ID: "tool", Kind: agent.BlockTool, Tool: &completed}) {
		t.Fatal("live tool was not completed in place")
	}
	if !tool.Expanded() {
		t.Fatal("live tool completion discarded the reader's expanded state")
	}
}

func TestTranscriptFocusSelectsAndOperatesOnOneEntry(t *testing.T) {
	view := testTranscriptView(t)
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
	view := newTranscriptView(kit.Dark(), kit.Unicode(), input.Wheel{}, highlight.New("github-dark"), 24, false, clipboard)
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

func TestDiscardExcessRetainsABoundedInteractiveWindow(t *testing.T) {
	view := testTranscriptView(t)
	view.retain = 2
	appendTestTool(view, "discarded", "OLD_DETAIL")
	for _, body := range []string{"second", "third", "fourth", "fifth"} {
		view.Append(&kit.Message{Theme: view.theme, Speaker: "lyra", Body: body})
	}
	root := headless.NewRoot(view)
	root.Draw(grid.NewSurface(48, 10).View())
	view.Focus(true)
	view.selectEntry(view.content.FirstBlock(), false)

	view.DiscardExcess()

	if view.content.Len() != 2 || view.content.FirstBlock() != 3 {
		t.Fatalf("retained transcript has %d blocks starting at %d, want 2 starting at 3", view.content.Len(), view.content.FirstBlock())
	}
	if len(view.entries) != 2 || len(view.toolViews) != 0 {
		t.Fatalf("discarded metadata survived: entries=%d tools=%d", len(view.entries), len(view.toolViews))
	}
	if !view.hasSelected || view.selected != 4 {
		t.Fatalf("selection after discard = present %t, id %d; want newest id 4", view.hasSelected, view.selected)
	}
	surface := grid.NewSurface(48, 10)
	root.Draw(surface.View())
	drawn := strings.Join(surface.Rows(), "\n")
	if strings.Contains(drawn, "third") || !strings.Contains(drawn, "fourth") || !strings.Contains(drawn, "fifth") {
		t.Fatalf("retained transcript rendered the wrong window:\n%s", drawn)
	}
}

type recordingClipboard struct{ text string }

func (c *recordingClipboard) Copy(value string) bool {
	c.text = value
	return true
}

func (*recordingClipboard) Paste() bool { return false }

func testTranscriptView(t *testing.T) *transcriptView {
	t.Helper()
	view := newTranscriptView(kit.Dark(), kit.Unicode(), input.Wheel{}, highlight.New("github-dark"), 24, false, nil)
	t.Cleanup(view.Close)
	return view
}

func appendTestTool(view *transcriptView, id, output string) *toolBlock {
	call := agent.ToolCall{Kind: agent.ToolShell, Command: "echo " + id, Output: output, Status: agent.ToolOK}
	block := newToolBlock(BlockPresentation{Theme: view.theme, Glyphs: view.glyphs, Look: view.look, Syntax: view.syntax}, agent.Block{
		ID: id, Kind: agent.BlockTool, Tool: &call,
	})
	blockID := view.place(block, true)
	view.toolViews = append(view.toolViews, trackedToolView{id: blockID, block: block})
	return block
}

func beginTestTool(view *transcriptView, call agent.ToolCall) *toolBlock {
	block := newToolBlock(BlockPresentation{Theme: view.theme, Glyphs: view.glyphs, Look: view.look, Syntax: view.syntax}, agent.Block{
		ID: "tool", Kind: agent.BlockTool, Tool: &call,
	})
	blockID := view.place(block, false)
	tracked := trackedTool{id: blockID, block: block}
	view.toolViews = append(view.toolViews, trackedToolView{id: blockID, block: block})
	view.tools["tool"] = liveTool{ids: []headless.BlockID{blockID}, blocks: []trackedTool{tracked}}
	return block
}
