package terminal

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/highlight"
	"github.com/Tangerg/oolong/markdown"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
)

type transcriptView struct {
	theme  kit.Theme
	glyphs kit.Glyphs
	wheel  input.Wheel
	look   markdown.Look
	syntax highlight.Renderer

	content         headless.Transcript
	scroll          headless.Scroll
	selection       headless.Selection
	sticky          headless.Sticky
	view            kit.Transcript
	search          *headless.Search
	query           string
	announceSearch  bool
	matches         []headless.Match
	current         int
	retain          int
	details         bool
	clipboard       headless.Clipboard
	focused         bool
	selected        headless.BlockID
	hasSelected     bool
	entries         map[headless.BlockID]*transcriptEntry
	pressed         headless.BlockID
	pressedHeader   bool
	dragged         bool
	onFocusChange   func(bool)
	onSelection     func(transcriptSelection)
	onCopy          func(string)
	keys            *keymap.Map
	matcher         keymap.Matcher
	tools           map[string]liveTool
	textStreams     map[string]*liveText
	toolViews       []trackedToolView
	runEntries      map[string][]headless.BlockID
	runLineages     map[string]agent.RunLineage
	activeToolGroup *trackedToolGroup
}

type transcriptSelection struct {
	Present    bool
	Readable   bool
	Expandable bool
	Expanded   bool
}

const transcriptPrompt keymap.Action = "prompt"

type liveTool struct {
	runID  string
	ids    []headless.BlockID
	blocks []trackedTool
	group  *trackedToolGroup
}

type liveText struct {
	runID  string
	stream markdown.Stream
	stable []markdown.Block
	block  *markdownBlock
	id     headless.BlockID
}

type trackedTool struct {
	id    headless.BlockID
	block mutableToolBlock
}

type trackedToolView struct {
	id    headless.BlockID
	block toolDisclosure
}

type trackedToolGroup struct {
	id    headless.BlockID
	runID string
	block *toolGroupBlock
}

func (c *transcriptView) ToggleDetails() {
	first := c.content.FirstBlock()
	// #nosec G115 -- Transcript.Len is non-negative and cannot exceed the
	// addressable in-memory slice backing the transcript.
	end := first + headless.BlockID(c.content.Len())
	expand, hasTool := false, false
	for _, tracked := range c.toolViews {
		if tracked.id < first || tracked.id >= end || !tracked.block.Expandable() {
			continue
		}
		hasTool = true
		if !tracked.block.Expanded() {
			expand = true
			break
		}
	}
	if !hasTool {
		expand = !c.details
	}
	c.details = expand
	for _, tracked := range c.toolViews {
		if tracked.id < first || tracked.id >= end || !tracked.block.Expandable() {
			continue
		}
		tracked.block.SetExpanded(c.details)
		c.content.Changed(tracked.id)
	}
	c.refreshSearch()
	c.announceSelection()
}

func (c *transcriptView) DetailsLabel() string {
	if c.details {
		return "tool details expanded"
	}
	return "tool details collapsed"
}

func newTranscriptView(
	theme kit.Theme,
	glyphs kit.Glyphs,
	wheel input.Wheel,
	syntax highlight.Renderer,
	retain int,
	details bool,
	clipboard headless.Clipboard,
) *transcriptView {
	c := &transcriptView{
		theme: theme, glyphs: glyphs, wheel: wheel,
		look: markdownLook(theme, glyphs, syntax), syntax: syntax,
		search: headless.NewSearch(), current: -1, retain: max(retain, 4), details: details,
		clipboard: clipboard, entries: make(map[headless.BlockID]*transcriptEntry),
		tools: make(map[string]liveTool), textStreams: make(map[string]*liveText),
		runEntries:  make(map[string][]headless.BlockID),
		runLineages: make(map[string]agent.RunLineage),
		keys:        transcriptKeys(),
	}
	c.scroll.Wheel(wheel)
	c.scroll.ToBottom()
	c.sticky.MinHeight, c.sticky.Gap = 1, 1
	c.view = kit.Transcript{
		Content: &c.content, Scroll: &c.scroll, Selection: &c.selection,
		Sticky: &c.sticky, Theme: theme, Glyphs: glyphs, Current: -1,
	}
	return c
}

func (c *transcriptView) Draw(frame headless.Frame) {
	c.view.Matches, c.view.Current = c.matches, c.current
	c.view.Draw(frame)
}

func (c *transcriptView) Handle(event input.Event) bool {
	if key, ok := event.(input.Key); ok && key.Down() && c.focused {
		if key.Code == input.Esc && c.selection.Active() {
			c.selection.Clear()
			return true
		}
		if _, handled := c.matcher.Handle(c.keys, key, c.Do); handled {
			return true
		}
	}
	handled := c.view.Handle(event)
	mouse, ok := event.(input.Mouse)
	if !handled || !ok {
		return handled
	}
	c.handleMouse(mouse)
	return true
}

func (c *transcriptView) Focus(has bool) {
	if c.focused == has {
		return
	}
	c.focused = has
	if !has {
		c.matcher.Clear()
	}
	if has {
		c.ensureSelection()
	}
	c.syncSelectedEntry()
	if c.onFocusChange != nil {
		c.onFocusChange(has)
	}
	c.announceSelection()
}

func (c *transcriptView) Focused() bool { return c.focused }

func (c *transcriptView) OnFocusChange(change func(bool)) { c.onFocusChange = change }

func (c *transcriptView) OnSelection(change func(transcriptSelection)) { c.onSelection = change }

func (c *transcriptView) OnCopy(copied func(string)) { c.onCopy = copied }

func (c *transcriptView) Keys() *keymap.Map { return c.keys }

func (c *transcriptView) action(event input.Event) keymap.Action {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return ""
	}
	action, _ := c.keys.Action(key.Chord())
	return action
}

func (c *transcriptView) Do(action keymap.Action) bool {
	switch action {
	case headless.SelectPrev:
		return c.moveSelection(-1)
	case headless.SelectNext:
		return c.moveSelection(1)
	case headless.SelectFirst:
		return c.selectEdge(false)
	case headless.SelectLast:
		return c.selectEdge(true)
	case headless.Collapse:
		return c.setSelectedExpanded(false)
	case headless.Expand:
		return c.setSelectedExpanded(true)
	case toggleDetails:
		return c.toggleSelected()
	case headless.Copy:
		return c.copySelected()
	}
	return false
}

func transcriptKeys() *keymap.Map {
	keys := &keymap.Map{}
	keys.Bind(headless.SelectPrev, input.Chord{Code: input.Up})
	keys.Bind(headless.SelectNext, input.Chord{Code: input.Down})
	keys.Bind(headless.SelectFirst, input.Chord{Code: input.Home})
	keys.Bind(headless.SelectLast, input.Chord{Code: input.End})
	keys.Bind(headless.Expand, input.Chord{Code: input.Right})
	keys.Bind(headless.Collapse, input.Chord{Code: input.Left})
	keys.Bind(toggleDetails, input.Chord{Code: input.Enter})
	keys.Bind(headless.Copy, input.Alt.Rune('c'))
	keys.Bind(openReader, input.Chord{Code: input.Character, Rune: 'v'})
	keys.Bind(transcriptPrompt, input.Chord{Code: input.Tab})
	keys.Bind(transcriptPrompt, input.Chord{Code: input.Character, Rune: ' '})
	keys.Bind(commandPalette, input.Chord{Code: input.Character, Rune: '?'})
	return keys
}

func (c *transcriptView) handleMouse(mouse input.Mouse) {
	if mouse.Button != input.ButtonLeft && mouse.Action != input.MouseUp {
		return
	}
	switch mouse.Action {
	case input.MouseDown:
		c.dragged = false
		point, _ := c.selection.Range()
		id, offset, ok := c.content.At(point.Row)
		if !ok {
			c.pressedHeader = false
			return
		}
		c.selectPointerEntry(id)
		c.pressed, c.pressedHeader = id, offset == 0 && c.tool(id) != nil
	case input.MouseDrag:
		c.dragged = true
	case input.MouseUp:
		start, end := c.selection.Range()
		id, offset, ok := c.content.At(end.Row)
		click := !c.dragged && start == end
		if click {
			c.selection.Clear()
		}
		if click && ok && id == c.pressed && offset == 0 && c.pressedHeader {
			c.toggleSelected()
		} else if !click {
			c.copySelection()
		}
		c.pressedHeader, c.dragged = false, false
	}
}

func (c *transcriptView) ensureSelection() {
	first := c.content.FirstBlock()
	if c.hasSelected && c.selected >= first && c.selected < first+blockOffset(c.content.Len()) {
		return
	}
	if c.content.Len() == 0 {
		c.hasSelected = false
		return
	}
	c.selected = first + blockOffset(c.content.Len()-1)
	c.hasSelected = true
	c.revealSelected()
}

func (c *transcriptView) moveSelection(delta int) bool {
	if c.content.Len() == 0 || delta == 0 {
		return false
	}
	c.ensureSelection()
	first := c.content.FirstBlock()
	last := first + blockOffset(c.content.Len()-1)
	next := c.selected
	switch {
	case delta < 0 && next > first:
		next--
	case delta > 0 && next < last:
		next++
	default:
		return true
	}
	c.selectEntry(next, true)
	return true
}

func (c *transcriptView) selectEdge(last bool) bool {
	if c.content.Len() == 0 {
		return false
	}
	id := c.content.FirstBlock()
	if last {
		id += blockOffset(c.content.Len() - 1)
	}
	c.selectEntry(id, true)
	return true
}

func (c *transcriptView) selectEntry(id headless.BlockID, reveal bool) {
	c.setSelectedEntry(id, reveal, true)
}

func (c *transcriptView) selectPointerEntry(id headless.BlockID) {
	c.setSelectedEntry(id, false, false)
}

func (c *transcriptView) setSelectedEntry(id headless.BlockID, reveal, clearTextSelection bool) {
	if _, ok := c.entries[id]; !ok {
		return
	}
	c.selected, c.hasSelected = id, true
	if clearTextSelection {
		c.selection.Clear()
	}
	c.syncSelectedEntry()
	if reveal {
		c.revealSelected()
	}
	c.announceSelection()
}

func (c *transcriptView) syncSelectedEntry() {
	for id, entry := range c.entries {
		entry.selected = c.hasSelected && id == c.selected
		entry.focused = entry.selected && c.focused
	}
}

func (c *transcriptView) revealSelected() {
	if !c.hasSelected {
		return
	}
	if top, height, ok := c.content.Extent(c.selected); ok {
		start := c.content.StartRow()
		c.scroll.RevealRange(top-start, top-start+height-1)
	}
}

func (c *transcriptView) tool(id headless.BlockID) toolDisclosure {
	for _, tracked := range c.toolViews {
		if tracked.id == id {
			return tracked.block
		}
	}
	return nil
}

func (c *transcriptView) toggleSelected() bool {
	tool := c.tool(c.selected)
	if !c.hasSelected || tool == nil || !tool.Expandable() {
		return true
	}
	expanded := tool.ToggleExpanded()
	c.content.Changed(c.selected)
	if expanded {
		c.revealSelected()
	}
	c.refreshSearch()
	c.announceSelection()
	return true
}

func (c *transcriptView) setSelectedExpanded(expanded bool) bool {
	tool := c.tool(c.selected)
	if !c.hasSelected || tool == nil || !tool.Expandable() {
		return true
	}
	if tool.Expanded() != expanded {
		tool.SetExpanded(expanded)
		c.content.Changed(c.selected)
		if expanded {
			c.revealSelected()
		}
		c.refreshSearch()
	}
	c.announceSelection()
	return true
}

func (c *transcriptView) copySelected() bool {
	if !c.hasSelected {
		return true
	}
	top, height, ok := c.content.Extent(c.selected)
	if !ok {
		return true
	}
	c.copy(copyableRowsText(c.content.Rows(top, height)))
	return true
}

func (c *transcriptView) copySelection() {
	c.copy(c.selection.Text(&c.content))
}

func (c *transcriptView) copy(value string) {
	if value == "" {
		return
	}
	if c.clipboard == nil || !c.clipboard.Copy(value) {
		return
	}
	if c.onCopy != nil {
		c.onCopy(value)
	}
}

func (c *transcriptView) announceSelection() {
	if c.onSelection == nil {
		return
	}
	_, readable := c.readerTargetForSelected()
	selection := transcriptSelection{Present: c.hasSelected, Readable: readable}
	if tool := c.tool(c.selected); tool != nil && tool.Expandable() {
		selection.Expandable = true
		selection.Expanded = tool.Expanded()
	}
	c.onSelection(selection)
}

func (c *transcriptView) selectedReaderTarget() (readerTarget, bool) {
	if !c.hasSelected {
		c.ensureSelection()
	}
	return c.readerTargetForSelected()
}

func (c *transcriptView) readerTargetForSelected() (readerTarget, bool) {
	if !c.hasSelected {
		return readerTarget{}, false
	}
	entry := c.entries[c.selected]
	if entry == nil || entry.content == nil {
		return readerTarget{}, false
	}
	switch tool := entry.content.(type) {
	case *toolBlock:
		return readerTarget{document: tool.readerDocument(), source: tool}, true
	case *toolGroupBlock:
		return readerTarget{document: tool.readerDocument(), source: tool}, true
	}
	copyable, ok := entry.content.(headless.Copyable)
	if !ok {
		return readerTarget{}, false
	}
	width := max(c.content.Width()-transcriptEntryInset, 40)
	value := copyableRowsText(copyable.Rows(width))
	if strings.TrimSpace(value) == "" {
		return readerTarget{}, false
	}
	title := "Transcript entry"
	switch block := entry.content.(type) {
	case *markdownBlock:
		title = block.speaker
	case *userMessageBlock:
		title = "you"
	case *kit.Message:
		if strings.TrimSpace(block.Speaker) != "" {
			title = block.Speaker
		}
	}
	return readerTarget{document: readerDocument{
		Title:    title,
		Sections: []ToolSection{{Style: toolSectionCode, Language: "text", Text: value}},
	}}, true
}

func (c *transcriptView) Follow() { c.scroll.ToBottom() }

func (c *transcriptView) Scroll(action keymap.Action) bool { return c.scroll.Do(action) }

func (c *transcriptView) Close() {
	if c != nil && c.search != nil {
		c.search.Close()
	}
}

func (c *transcriptView) Apply(event agent.Event, registry *extensions.Registry) error {
	return c.apply("", event, registry)
}

func (c *transcriptView) ApplyRunEvent(envelope agent.RunEvent, registry *extensions.Registry) error {
	if started, ok := envelope.Event.(agent.SegmentStarted); ok {
		c.runLineages[started.Run.ID] = started.Run.Lineage
	}
	return c.apply(envelope.RunID, envelope.Event, registry)
}

func (c *transcriptView) apply(runID string, event agent.Event, registry *extensions.Registry) error {
	switch e := event.(type) {
	case agent.BlockStarted:
		if e.Block.Kind == agent.BlockAssistant || e.Block.Kind == agent.BlockReasoning {
			return c.begin(e.Block)
		}
		if e.Block.Kind == agent.BlockTool {
			return c.beginTool(e.Block, registry)
		}
		c.sealToolGroup()
	case agent.BlockDelta:
		key := transcriptBlockKey(runID, e.BlockID)
		if _, live := c.tools[key]; live {
			return c.deltaTool(key, e.BlockID, e.Text)
		}
		return c.delta(key, e.BlockID, e.Text)
	case agent.ToolArgumentsDelta, agent.RunProgress:
		// Tool arguments are provisional JSON and progress belongs in the status
		// chrome. Neither creates an authoritative transcript block.
	case agent.CustomEvent:
		return c.appendCustom(runID, e, registry)
	case agent.BlockCompleted:
		return c.complete(e.Block, registry)
	case agent.RunFinished:
		if strings.TrimSpace(runID) == "" {
			c.settleLive(e.Outcome)
		} else {
			c.settleRun(runID, e.Outcome)
		}
	case agent.RunInterrupted:
		c.sealToolGroup()
	}
	return nil
}

func (c *transcriptView) appendCustom(runID string, event agent.CustomEvent, registry *extensions.Registry) error {
	for _, presenter := range extensions.Values(registry, CustomEventPresenters) {
		if presenter.Name != event.Name {
			continue
		}
		rendered, err := presentCustomSafely(presenter, BlockPresentation{
			Theme: c.theme, Glyphs: c.glyphs, Look: c.look, Syntax: c.syntax,
			Tools: extensions.Values(registry, ToolPresenters), Speaker: "runtime",
		}, event)
		if err != nil {
			return err
		}
		c.sealToolGroup()
		for _, block := range rendered {
			id := c.append(block)
			c.trackRunEntry(runID, id)
		}
		return nil
	}
	return nil
}

func presentCustomSafely(presenter CustomEventPresenter, presentation BlockPresentation, event agent.CustomEvent) (rendered []headless.Block, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("terminal transcript: custom presenter for %q panicked: %v", presenter.Name, recovered)
		}
	}()
	return presenter.Present(presentation, event), nil
}

func (c *transcriptView) begin(block agent.Block) error {
	key := transcriptBlockKey(block.RunID, block.ID)
	if _, exists := c.textStreams[key]; exists {
		return fmt.Errorf("terminal transcript: text block %s started twice", block.ID)
	}
	if _, exists := c.tools[key]; exists {
		return fmt.Errorf("terminal transcript: block %s is already a live tool", block.ID)
	}
	c.sealToolGroup()
	speaker := c.speakerFor(block)
	live := &liveText{runID: block.RunID, block: &markdownBlock{theme: c.theme, speaker: speaker}}
	live.stream.SetLook(c.lookFor(block.Kind))
	live.id = c.place(live.block, false)
	c.trackRunEntry(block.RunID, live.id)
	c.textStreams[key] = live
	if block.Text != "" {
		return c.delta(key, block.ID, block.Text)
	}
	return nil
}

func (c *transcriptView) delta(key, blockID, chunk string) error {
	live, ok := c.textStreams[key]
	if !ok {
		return fmt.Errorf("terminal transcript: delta for inactive text block %s", blockID)
	}
	live.stable = append(live.stable, live.stream.Feed(chunk)...)
	blocks := slices.Clone(live.stable)
	blocks = append(blocks, live.stream.Open()...)
	live.block.doc.SetBlocks(blocks)
	c.content.Changed(live.id)
	c.refreshSearch()
	return nil
}

func (c *transcriptView) deltaTool(key, blockID, chunk string) error {
	live, ok := c.tools[key]
	if !ok {
		return fmt.Errorf("terminal transcript: delta for inactive tool block %s", blockID)
	}
	for _, tracked := range live.blocks {
		tracked.block.AppendOutput(chunk)
		c.content.Changed(tracked.id)
	}
	c.refreshSearch()
	c.announceSelection()
	return nil
}

func (c *transcriptView) complete(block agent.Block, registry *extensions.Registry) error {
	key := transcriptBlockKey(block.RunID, block.ID)
	if _, live := c.textStreams[key]; live {
		return c.completeStream(block)
	}
	if block.Kind == agent.BlockTool && c.completeLiveTool(block) {
		return nil
	}
	return c.appendCompleted(block, registry)
}

func (c *transcriptView) completeStream(block agent.Block) error {
	key := transcriptBlockKey(block.RunID, block.ID)
	live, ok := c.textStreams[key]
	if !ok {
		return fmt.Errorf("terminal transcript: completion for inactive text block %s", block.ID)
	}
	// The completed value is authoritative. Re-rendering it once also repairs a
	// transport that intentionally replaced an earlier provisional tail.
	live.block.doc.SetBlocks(markdown.Render(block.Text, c.lookFor(block.Kind)))
	c.content.Changed(live.id)
	c.content.Finish(live.id)
	live.stream.Reset()
	delete(c.textStreams, key)
	c.refreshSearch()
	return nil
}

func (c *transcriptView) completeLiveTool(block agent.Block) bool {
	key := transcriptBlockKey(block.RunID, block.ID)
	live, ok := c.tools[key]
	if !ok {
		return false
	}
	for _, tracked := range live.blocks {
		tracked.block.Update(block)
		c.content.Changed(tracked.id)
	}
	if live.group != nil {
		c.finishToolGroupIfReady(live.group)
	} else {
		for _, id := range live.ids {
			c.content.Finish(id)
		}
	}
	delete(c.tools, key)
	if len(live.blocks) == 0 {
		return false
	}
	c.refreshSearch()
	c.announceSelection()
	return true
}

func (c *transcriptView) settleLive(outcome agent.Outcome) {
	for id, live := range c.textStreams {
		c.content.Finish(live.id)
		live.stream.Reset()
		delete(c.textStreams, id)
	}
	toolStatus := agent.ToolError
	if outcome.Status == agent.OutcomeCanceled {
		toolStatus = agent.ToolCanceled
	}
	for id, live := range c.tools {
		for _, tracked := range live.blocks {
			tracked.block.Finish(toolStatus)
			c.content.Changed(tracked.id)
		}
		if live.group != nil {
			c.finishToolGroupIfReady(live.group)
		} else {
			for _, blockID := range live.ids {
				c.content.Finish(blockID)
			}
		}
		delete(c.tools, id)
	}
	c.sealToolGroup()
	c.refreshSearch()
	c.announceSelection()
}

func (c *transcriptView) settleRun(runID string, outcome agent.Outcome) {
	for id, live := range c.textStreams {
		if live.runID != runID {
			continue
		}
		c.content.Finish(live.id)
		live.stream.Reset()
		delete(c.textStreams, id)
	}
	toolStatus := agent.ToolError
	if outcome.Status == agent.OutcomeCanceled {
		toolStatus = agent.ToolCanceled
	}
	for id, live := range c.tools {
		if live.runID != runID {
			continue
		}
		for _, tracked := range live.blocks {
			tracked.block.Finish(toolStatus)
			c.content.Changed(tracked.id)
		}
		if live.group != nil {
			c.finishToolGroupIfReady(live.group)
		} else {
			for _, blockID := range live.ids {
				c.content.Finish(blockID)
			}
		}
		delete(c.tools, id)
	}
	if c.activeToolGroup != nil && c.activeToolGroup.runID == runID {
		c.sealToolGroup()
	}
	c.refreshSearch()
	c.announceSelection()
}

func (c *transcriptView) appendCompleted(block agent.Block, registry *extensions.Registry) error {
	rendered, err := c.present(block, registry)
	if err != nil {
		return err
	}
	if block.Kind == agent.BlockTool {
		if tool, grouped := groupedTool(rendered); grouped {
			c.addGroupedTool(block.RunID, tool)
			c.refreshSearch()
			return nil
		}
	}
	c.sealToolGroup()
	for _, item := range rendered {
		mutable, isMutable := item.(mutableToolBlock)
		if isMutable {
			mutable.SetExpanded(c.details)
		}
		id := c.append(item)
		c.trackRunEntry(block.RunID, id)
		if isMutable {
			c.toolViews = append(c.toolViews, trackedToolView{id: id, block: mutable})
		}
		if block.Kind == agent.BlockUser {
			c.sticky.Add(id)
		}
	}
	return nil
}

func (c *transcriptView) beginTool(block agent.Block, registry *extensions.Registry) error {
	key := transcriptBlockKey(block.RunID, block.ID)
	if _, exists := c.tools[key]; exists {
		return fmt.Errorf("terminal transcript: tool block %s started twice", block.ID)
	}
	if _, exists := c.textStreams[key]; exists {
		return fmt.Errorf("terminal transcript: block %s is already a live text block", block.ID)
	}
	rendered, err := c.present(block, registry)
	if err != nil {
		return err
	}
	if tool, grouped := groupedTool(rendered); grouped {
		group := c.addGroupedTool(block.RunID, tool)
		tracked := trackedTool{id: group.id, block: tool}
		c.tools[key] = liveTool{runID: block.RunID, blocks: []trackedTool{tracked}, group: group}
		c.refreshSearch()
		return nil
	}
	c.sealToolGroup()
	live := liveTool{runID: block.RunID}
	for _, item := range rendered {
		mutable, isMutable := item.(mutableToolBlock)
		if isMutable {
			mutable.SetExpanded(c.details)
		}
		id := c.place(item, false)
		c.trackRunEntry(block.RunID, id)
		live.ids = append(live.ids, id)
		if isMutable {
			tracked := trackedTool{id: id, block: mutable}
			live.blocks = append(live.blocks, tracked)
			c.toolViews = append(c.toolViews, trackedToolView{id: id, block: mutable})
		}
	}
	c.tools[key] = live
	c.refreshSearch()
	return nil
}

func groupedTool(rendered []headless.Block) (*toolBlock, bool) {
	if len(rendered) != 1 {
		return nil, false
	}
	tool, ok := rendered[0].(*toolBlock)
	return tool, ok && groupableTool(tool.call)
}

func (c *transcriptView) addGroupedTool(runID string, tool *toolBlock) *trackedToolGroup {
	group := c.activeToolGroup
	if group == nil || group.runID != runID {
		c.sealToolGroup()
		block := newToolGroupBlock(c.theme, c.glyphs, c.details)
		block.Add(tool)
		group = &trackedToolGroup{runID: runID, block: block}
		group.id = c.place(block, false)
		c.trackRunEntry(runID, group.id)
		c.toolViews = append(c.toolViews, trackedToolView{id: group.id, block: block})
		c.activeToolGroup = group
		return group
	}
	group.block.Add(tool)
	c.content.Changed(group.id)
	return group
}

func (c *transcriptView) sealToolGroup() {
	group := c.activeToolGroup
	if group == nil {
		return
	}
	group.block.Seal()
	c.content.Changed(group.id)
	c.activeToolGroup = nil
	c.finishToolGroupIfReady(group)
}

func (c *transcriptView) finishToolGroupIfReady(group *trackedToolGroup) {
	if group != nil && group.block.ReadyToFinish() {
		c.content.Finish(group.id)
	}
}

// SealToolGroups closes the trailing adjacency window after a cold snapshot.
// A live event stream closes it naturally on the next semantic boundary.
func (c *transcriptView) SealToolGroups() { c.sealToolGroup() }

func (c *transcriptView) present(block agent.Block, registry *extensions.Registry) ([]headless.Block, error) {
	for _, presenter := range extensions.Values(registry, BlockPresenters) {
		if presenter.Kind == block.Kind {
			return presentSafely(presenter, BlockPresentation{
				Theme: c.theme, Glyphs: c.glyphs, Look: c.look, Syntax: c.syntax,
				Tools: extensions.Values(registry, ToolPresenters), Speaker: c.speakerFor(block),
			}, block)
		}
	}
	return nil, fmt.Errorf("terminal transcript: no presenter for block kind %q", block.Kind)
}

func presentSafely(presenter BlockPresenter, presentation BlockPresentation, block agent.Block) (rendered []headless.Block, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("terminal transcript: presenter for %q panicked: %v", presenter.Kind, recovered)
		}
	}()
	return presenter.Present(presentation, block), nil
}

func (c *transcriptView) Append(block headless.Block) {
	c.sealToolGroup()
	c.append(block)
}

func (c *transcriptView) append(block headless.Block) headless.BlockID {
	id := c.place(block, true)
	c.refreshSearch()
	return id
}

func (c *transcriptView) place(block headless.Block, finished bool) headless.BlockID {
	entry := newTranscriptEntry(c.theme, c.glyphs, block)
	id := c.content.Append(entry)
	c.entries[id] = entry
	if finished {
		c.content.Finish(id)
	}
	return id
}

type discardedOutput struct{}

func (discardedOutput) Print(grid.Drawable) {}

func (c *transcriptView) DiscardExcess() {
	if c.content.Width() <= 0 {
		return
	}
	finished := 0
	for i := range c.content.Len() {
		id := c.content.FirstBlock() + headless.BlockID(i)
		if !c.content.Finished(id) {
			break
		}
		finished++
	}
	if excess := finished - c.retain; excess > 0 {
		c.view.Commit(discardedOutput{}, excess)
	}
	first := c.content.FirstBlock()
	c.toolViews = slices.DeleteFunc(c.toolViews, func(item trackedToolView) bool { return item.id < first })
	for id := range c.entries {
		if id < first {
			delete(c.entries, id)
		}
	}
	for runID, ids := range c.runEntries {
		ids = slices.DeleteFunc(ids, func(id headless.BlockID) bool { return id < first })
		if len(ids) == 0 {
			delete(c.runEntries, runID)
		} else {
			c.runEntries[runID] = ids
		}
	}
	if c.hasSelected && c.selected < first {
		c.hasSelected = false
		c.ensureSelection()
		c.syncSelectedEntry()
		c.announceSelection()
	}
	c.refreshSearch()
}

func (c *transcriptView) Reset() {
	c.content = headless.Transcript{}
	c.scroll = headless.Scroll{}
	c.scroll.Wheel(c.wheel)
	c.scroll.ToBottom()
	c.selection = headless.Selection{}
	c.sticky = headless.Sticky{MinHeight: 1, Gap: 1}
	c.view.Content, c.view.Scroll = &c.content, &c.scroll
	c.view.Selection, c.view.Sticky = &c.selection, &c.sticky
	for _, live := range c.textStreams {
		live.stream.Reset()
	}
	clear(c.textStreams)
	c.query, c.matches, c.current, c.announceSearch = "", nil, -1, false
	clear(c.tools)
	clear(c.entries)
	clear(c.runEntries)
	clear(c.runLineages)
	c.activeToolGroup = nil
	c.hasSelected, c.pressedHeader, c.dragged = false, false, false
	c.toolViews = nil
	c.search.Submit(&c.content, "", false)
}

func (c *transcriptView) SetRuns(runs []agent.Run) {
	clear(c.runLineages)
	for _, run := range runs {
		c.runLineages[run.ID] = run.Lineage
	}
}

func (c *transcriptView) speakerFor(block agent.Block) string {
	lineage, known := c.runLineages[block.RunID]
	if !known || lineage.IsRoot() {
		switch block.Kind {
		case agent.BlockUser:
			return "you"
		case agent.BlockReasoning:
			return "thinking"
		default:
			return "lyra"
		}
	}
	identity := shortIdentity(block.RunID)
	switch block.Kind {
	case agent.BlockUser:
		return "subagent input · " + identity
	case agent.BlockReasoning:
		return "subagent thinking · " + identity
	default:
		return "subagent · " + identity
	}
}

func (c *transcriptView) trackRunEntry(runID string, id headless.BlockID) {
	if strings.TrimSpace(runID) == "" {
		return
	}
	c.runEntries[runID] = append(c.runEntries[runID], id)
}

func (c *transcriptView) JumpToRun(runID string) bool {
	first := c.content.FirstBlock()
	last := first + blockOffset(c.content.Len())
	for _, id := range c.runEntries[runID] {
		if id < first || id >= last {
			continue
		}
		c.selectEntry(id, true)
		return true
	}
	return false
}

func blockOffset(index int) headless.BlockID {
	if index < 0 {
		panic("terminal: negative transcript block offset")
	}
	return headless.BlockID(index) // #nosec G115 -- validated nonnegative and int cannot exceed uint64.
}

func transcriptBlockKey(runID, blockID string) string {
	if runID == "" {
		return blockID
	}
	return runID + "\x00" + blockID
}

func (c *transcriptView) Find(query string) {
	c.query = strings.TrimSpace(query)
	c.announceSearch = c.query != ""
	c.matches, c.current = nil, -1
	c.search.Submit(&c.content, c.query, false)
}

func (c *transcriptView) refreshSearch() {
	if c.query != "" {
		c.search.Submit(&c.content, c.query, false)
	}
}

func (c *transcriptView) SearchResults() <-chan headless.Result { return c.search.Results() }

func (c *transcriptView) AcceptSearch(result headless.Result) (accepted, announce bool) {
	if result.Query != c.query {
		return false, false
	}
	c.matches = result.Matches
	if len(c.matches) > 0 {
		c.current = 0
	} else {
		c.current = -1
	}
	announce, c.announceSearch = c.announceSearch, false
	return true, announce
}

func (c *transcriptView) StepMatch(delta int) bool {
	if len(c.matches) == 0 {
		return false
	}
	c.current = (c.current + delta) % len(c.matches)
	if c.current < 0 {
		c.current += len(c.matches)
	}
	return true
}

func (c *transcriptView) lookFor(kind agent.BlockKind) markdown.Look {
	look := c.look
	if kind == agent.BlockReasoning {
		look.Text, look.Strong, look.Code = c.theme.Muted, c.theme.Subtle, c.theme.Info
	}
	return look
}
