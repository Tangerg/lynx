package terminal

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/highlight"
	"github.com/Tangerg/oolong/markdown"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
)

type conversationView struct {
	theme  kit.Theme
	glyphs kit.Glyphs
	wheel  input.Wheel
	look   markdown.Look
	syntax highlight.Style

	content        headless.Transcript
	scroll         headless.Scroll
	selection      headless.Selection
	sticky         headless.Sticky
	view           kit.Transcript
	search         *headless.Search
	query          string
	announceSearch bool
	matches        []headless.Match
	current        int
	retain         int
	details        bool
	clipboard      headless.Clipboard
	focused        bool
	selected       headless.BlockID
	hasSelected    bool
	entries        map[headless.BlockID]*transcriptEntry
	pressed        headless.BlockID
	pressedHeader  bool
	dragged        bool
	onFocusChange  func(bool)
	onSelection    func(transcriptSelection)
	onCopy         func(string)
	keys           *keymap.Map
	matcher        keymap.Matcher
	tools          map[string]liveTool
	textStreams    map[string]*liveText
	toolViews      []trackedTool
}

type transcriptSelection struct {
	Present    bool
	Expandable bool
	Expanded   bool
}

const transcriptPrompt keymap.Action = "prompt"

type liveTool struct {
	ids    []headless.BlockID
	blocks []trackedTool
}

type liveText struct {
	stream markdown.Stream
	stable []markdown.Block
	block  *markdownBlock
	id     headless.BlockID
}

type trackedTool struct {
	id    headless.BlockID
	block mutableToolBlock
}

func (c *conversationView) ToggleDetails() {
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

func (c *conversationView) DetailsLabel() string {
	if c.details {
		return "tool details expanded"
	}
	return "tool details collapsed"
}

func newConversationView(
	theme kit.Theme,
	glyphs kit.Glyphs,
	wheel input.Wheel,
	syntax highlight.Style,
	retain int,
	details bool,
	clipboard headless.Clipboard,
) *conversationView {
	c := &conversationView{
		theme: theme, glyphs: glyphs, wheel: wheel,
		look: markdownLook(theme, glyphs, syntax), syntax: syntax,
		search: headless.NewSearch(), current: -1, retain: max(retain, 4), details: details,
		clipboard: clipboard, entries: make(map[headless.BlockID]*transcriptEntry),
		tools: make(map[string]liveTool), textStreams: make(map[string]*liveText),
		keys: transcriptKeys(),
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

func (c *conversationView) Draw(frame headless.Frame) {
	c.view.Matches, c.view.Current = c.matches, c.current
	c.view.Draw(frame)
}

func (c *conversationView) Handle(event input.Event) bool {
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

func (c *conversationView) Focus(has bool) {
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

func (c *conversationView) Focused() bool { return c.focused }

func (c *conversationView) OnFocusChange(change func(bool)) { c.onFocusChange = change }

func (c *conversationView) OnSelection(change func(transcriptSelection)) { c.onSelection = change }

func (c *conversationView) OnCopy(copied func(string)) { c.onCopy = copied }

func (c *conversationView) Keys() *keymap.Map { return c.keys }

func (c *conversationView) Do(action keymap.Action) bool {
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
	keys.Bind(transcriptPrompt, input.Chord{Code: input.Tab})
	keys.Bind(transcriptPrompt, input.Chord{Code: input.Character, Rune: ' '})
	return keys
}

func (c *conversationView) handleMouse(mouse input.Mouse) {
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

func (c *conversationView) ensureSelection() {
	first := c.content.FirstBlock()
	if c.hasSelected && c.selected >= first && c.selected < first+headless.BlockID(c.content.Len()) {
		return
	}
	if c.content.Len() == 0 {
		c.hasSelected = false
		return
	}
	c.selected = first + headless.BlockID(c.content.Len()-1)
	c.hasSelected = true
	c.revealSelected()
}

func (c *conversationView) moveSelection(delta int) bool {
	if c.content.Len() == 0 || delta == 0 {
		return false
	}
	c.ensureSelection()
	first := c.content.FirstBlock()
	last := first + headless.BlockID(c.content.Len()-1)
	next := c.selected
	if delta < 0 && next > first {
		next--
	} else if delta > 0 && next < last {
		next++
	} else {
		return true
	}
	c.selectEntry(next, true)
	return true
}

func (c *conversationView) selectEdge(last bool) bool {
	if c.content.Len() == 0 {
		return false
	}
	id := c.content.FirstBlock()
	if last {
		id += headless.BlockID(c.content.Len() - 1)
	}
	c.selectEntry(id, true)
	return true
}

func (c *conversationView) selectEntry(id headless.BlockID, reveal bool) {
	c.setSelectedEntry(id, reveal, true)
}

func (c *conversationView) selectPointerEntry(id headless.BlockID) {
	c.setSelectedEntry(id, false, false)
}

func (c *conversationView) setSelectedEntry(id headless.BlockID, reveal, clearTextSelection bool) {
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

func (c *conversationView) syncSelectedEntry() {
	for id, entry := range c.entries {
		entry.selected = c.hasSelected && id == c.selected
		entry.focused = entry.selected && c.focused
	}
}

func (c *conversationView) revealSelected() {
	if !c.hasSelected {
		return
	}
	if top, height, ok := c.content.Extent(c.selected); ok {
		start := c.content.StartRow()
		c.scroll.RevealRange(top-start, top-start+height-1)
	}
}

func (c *conversationView) tool(id headless.BlockID) mutableToolBlock {
	for _, tracked := range c.toolViews {
		if tracked.id == id {
			return tracked.block
		}
	}
	return nil
}

func (c *conversationView) toggleSelected() bool {
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

func (c *conversationView) setSelectedExpanded(expanded bool) bool {
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

func (c *conversationView) copySelected() bool {
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

func (c *conversationView) copySelection() {
	c.copy(c.selection.Text(&c.content))
}

func (c *conversationView) copy(value string) {
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

func (c *conversationView) announceSelection() {
	if c.onSelection == nil {
		return
	}
	selection := transcriptSelection{Present: c.hasSelected}
	if tool := c.tool(c.selected); tool != nil && tool.Expandable() {
		selection.Expandable = true
		selection.Expanded = tool.Expanded()
	}
	c.onSelection(selection)
}

func (c *conversationView) Follow() { c.scroll.ToBottom() }

func (c *conversationView) Scroll(action keymap.Action) bool { return c.scroll.Do(action) }

func (c *conversationView) Close() {
	if c != nil && c.search != nil {
		c.search.Close()
	}
}

func (c *conversationView) Apply(event agent.Event, registry *extensions.Registry) error {
	switch e := event.(type) {
	case agent.BlockStarted:
		if e.Block.Kind == agent.BlockAssistant || e.Block.Kind == agent.BlockReasoning {
			return c.begin(e.Block)
		}
		if e.Block.Kind == agent.BlockTool {
			return c.beginTool(e.Block, registry)
		}
	case agent.BlockDelta:
		if _, live := c.tools[e.BlockID]; live {
			return c.deltaTool(e.BlockID, e.Text)
		}
		return c.delta(e.BlockID, e.Text)
	case agent.BlockCompleted:
		return c.complete(e.Block, registry)
	case agent.RunFinished:
		c.settleLive(e.Outcome)
	}
	return nil
}

func (c *conversationView) begin(block agent.Block) error {
	if _, exists := c.textStreams[block.ID]; exists {
		return fmt.Errorf("terminal transcript: text block %s started twice", block.ID)
	}
	if _, exists := c.tools[block.ID]; exists {
		return fmt.Errorf("terminal transcript: block %s is already a live tool", block.ID)
	}
	speaker := "lyra"
	if block.Kind == agent.BlockReasoning {
		speaker = "thinking"
	}
	live := &liveText{block: &markdownBlock{theme: c.theme, speaker: speaker}}
	live.stream.SetLook(c.lookFor(block.Kind))
	live.id = c.place(live.block, false)
	c.textStreams[block.ID] = live
	if block.Text != "" {
		return c.delta(block.ID, block.Text)
	}
	return nil
}

func (c *conversationView) delta(id, chunk string) error {
	live, ok := c.textStreams[id]
	if !ok {
		return fmt.Errorf("terminal transcript: delta for inactive text block %s", id)
	}
	live.stable = append(live.stable, live.stream.Feed(chunk)...)
	blocks := append([]markdown.Block(nil), live.stable...)
	blocks = append(blocks, live.stream.Open()...)
	live.block.doc.SetBlocks(blocks)
	c.content.Changed(live.id)
	c.refreshSearch()
	return nil
}

func (c *conversationView) deltaTool(id, chunk string) error {
	live, ok := c.tools[id]
	if !ok {
		return fmt.Errorf("terminal transcript: delta for inactive tool block %s", id)
	}
	for _, tracked := range live.blocks {
		tracked.block.AppendOutput(chunk)
		c.content.Changed(tracked.id)
	}
	c.refreshSearch()
	c.announceSelection()
	return nil
}

func (c *conversationView) complete(block agent.Block, registry *extensions.Registry) error {
	if _, live := c.textStreams[block.ID]; live {
		return c.completeStream(block)
	}
	if block.Kind == agent.BlockTool && c.completeLiveTool(block) {
		return nil
	}
	return c.appendCompleted(block, registry)
}

func (c *conversationView) completeStream(block agent.Block) error {
	live, ok := c.textStreams[block.ID]
	if !ok {
		return fmt.Errorf("terminal transcript: completion for inactive text block %s", block.ID)
	}
	// The completed value is authoritative. Re-rendering it once also repairs a
	// transport that intentionally replaced an earlier provisional tail.
	live.block.doc.SetBlocks(markdown.Render(block.Text, c.lookFor(block.Kind)))
	c.content.Changed(live.id)
	c.content.Finish(live.id)
	live.stream.Reset()
	delete(c.textStreams, block.ID)
	c.refreshSearch()
	return nil
}

func (c *conversationView) completeLiveTool(block agent.Block) bool {
	live, ok := c.tools[block.ID]
	if !ok {
		return false
	}
	for _, tracked := range live.blocks {
		tracked.block.Update(block)
		c.content.Changed(tracked.id)
	}
	for _, id := range live.ids {
		c.content.Finish(id)
	}
	delete(c.tools, block.ID)
	if len(live.blocks) == 0 {
		return false
	}
	c.refreshSearch()
	c.announceSelection()
	return true
}

func (c *conversationView) settleLive(outcome agent.Outcome) {
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
		for _, blockID := range live.ids {
			c.content.Finish(blockID)
		}
		delete(c.tools, id)
	}
	c.refreshSearch()
	c.announceSelection()
}

func (c *conversationView) appendCompleted(block agent.Block, registry *extensions.Registry) error {
	rendered, err := c.present(block, registry)
	if err != nil {
		return err
	}
	for _, item := range rendered {
		mutable, isMutable := item.(mutableToolBlock)
		if isMutable {
			mutable.SetExpanded(c.details)
		}
		id := c.append(item)
		if isMutable {
			c.toolViews = append(c.toolViews, trackedTool{id: id, block: mutable})
		}
		if block.Kind == agent.BlockUser {
			c.sticky.Add(id)
		}
	}
	return nil
}

func (c *conversationView) beginTool(block agent.Block, registry *extensions.Registry) error {
	if _, exists := c.tools[block.ID]; exists {
		return fmt.Errorf("terminal transcript: tool block %s started twice", block.ID)
	}
	if _, exists := c.textStreams[block.ID]; exists {
		return fmt.Errorf("terminal transcript: block %s is already a live text block", block.ID)
	}
	rendered, err := c.present(block, registry)
	if err != nil {
		return err
	}
	live := liveTool{}
	for _, item := range rendered {
		mutable, isMutable := item.(mutableToolBlock)
		if isMutable {
			mutable.SetExpanded(c.details)
		}
		id := c.place(item, false)
		live.ids = append(live.ids, id)
		if isMutable {
			tracked := trackedTool{id: id, block: mutable}
			live.blocks = append(live.blocks, tracked)
			c.toolViews = append(c.toolViews, tracked)
		}
	}
	c.tools[block.ID] = live
	c.refreshSearch()
	return nil
}

func (c *conversationView) present(block agent.Block, registry *extensions.Registry) ([]headless.Block, error) {
	for _, presenter := range extensions.Values(registry, BlockPresenters) {
		if presenter.Kind == block.Kind {
			return presentSafely(presenter, Presentation{
				Theme: c.theme, Glyphs: c.glyphs, Look: c.look, Syntax: c.syntax,
			}, block)
		}
	}
	return nil, fmt.Errorf("terminal transcript: no presenter for block kind %q", block.Kind)
}

func presentSafely(presenter BlockPresenter, presentation Presentation, block agent.Block) (rendered []headless.Block, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("terminal transcript: presenter for %q panicked: %v", presenter.Kind, recovered)
		}
	}()
	return presenter.Present(presentation, block), nil
}

func (c *conversationView) Append(block headless.Block) { c.append(block) }

func (c *conversationView) append(block headless.Block) headless.BlockID {
	id := c.place(block, true)
	c.refreshSearch()
	return id
}

func (c *conversationView) place(block headless.Block, finished bool) headless.BlockID {
	entry := newTranscriptEntry(c.theme, c.glyphs, block)
	id := c.content.Append(entry)
	c.entries[id] = entry
	if finished {
		c.content.Finish(id)
	}
	return id
}

func (c *conversationView) Retain(printer kit.Printer) {
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
		c.view.CommitN(printer, excess)
	}
	first := c.content.FirstBlock()
	c.toolViews = slices.DeleteFunc(c.toolViews, func(item trackedTool) bool { return item.id < first })
	for id := range c.entries {
		if id < first {
			delete(c.entries, id)
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

func (c *conversationView) Reset() {
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
	c.hasSelected, c.pressedHeader, c.dragged = false, false, false
	c.toolViews = nil
	c.search.Submit(&c.content, "", false)
}

func (c *conversationView) Find(query string) {
	c.query = strings.TrimSpace(query)
	c.announceSearch = c.query != ""
	c.matches, c.current = nil, -1
	c.search.Submit(&c.content, c.query, false)
}

func (c *conversationView) refreshSearch() {
	if c.query != "" {
		c.search.Submit(&c.content, c.query, false)
	}
}

func (c *conversationView) SearchResults() <-chan headless.Result { return c.search.Results() }

func (c *conversationView) AcceptSearch(result headless.Result) (accepted, announce bool) {
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

func (c *conversationView) StepMatch(delta int) bool {
	if len(c.matches) == 0 {
		return false
	}
	c.current = (c.current + delta) % len(c.matches)
	if c.current < 0 {
		c.current += len(c.matches)
	}
	return true
}

func (c *conversationView) lookFor(kind agent.BlockKind) markdown.Look {
	look := c.look
	if kind == agent.BlockReasoning {
		look.Text, look.Strong, look.Code = c.theme.Muted, c.theme.Subtle, c.theme.Info
	}
	return look
}
