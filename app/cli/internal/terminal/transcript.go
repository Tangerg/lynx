package terminal

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
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

	content          headless.Transcript
	scroll           headless.Scroll
	selection        headless.Selection
	sticky           headless.Sticky
	view             kit.Transcript
	search           *headless.Search
	query            string
	announceSearch   bool
	matches          []headless.Match
	current          int
	searchCursor     transcriptSearchCursor
	retain           int
	details          bool
	clipboard        headless.Clipboard
	entrance         grid.Drawable
	focused          bool
	selected         headless.BlockID
	hasSelected      bool
	entries          map[headless.BlockID]*transcriptEntry
	pointerGesture   transcriptPointerGesture
	onFocusChange    func(bool)
	onSelection      func(transcriptSelection)
	onCopy           func(string)
	keys             *keymap.Map
	matcher          keymap.Matcher
	tools            map[string]liveTool
	textStreams      map[string]*liveText
	pendingQuestions map[string]trackedQuestion
	toolViews        []trackedToolView
	runEntries       map[string][]headless.BlockID
	runLineages      map[string]agent.RunLineage
	activeToolGroup  *trackedToolGroup
	images           *terminalImagePresenter
	contentEpoch     uint64
	presentedBlocks  headless.Snapshot[transcriptBlockPresentation]
}

type transcriptSelection struct {
	Present    bool
	Readable   bool
	Expandable bool
	Expanded   bool
}

type transcriptBlockPlacement struct {
	blockID     headless.BlockID
	top, height int
}

type transcriptBlockPresentation struct {
	epoch  uint64
	blocks []transcriptBlockPlacement
}

type transcriptPointerGesture struct {
	target  headless.BlockID
	header  bool
	dragged bool
}

func (t *transcriptPointerGesture) begin(target headless.BlockID, header bool) {
	*t = transcriptPointerGesture{target: target, header: header}
}

func (t *transcriptPointerGesture) drag() { t.dragged = true }

func (t *transcriptPointerGesture) release(
	selected headless.BlockID,
	selectedPresent bool,
	selectionCollapsed bool,
) (click, activate bool) {
	click = !t.dragged && selectionCollapsed
	activate = click && selectedPresent && t.header && t.target == selected
	t.cancel()
	return click, activate
}

func (t *transcriptPointerGesture) cancel() { *t = transcriptPointerGesture{} }

type transcriptSearchCursor struct {
	blockID           headless.BlockID
	rowOffset, column int
	index             int
	present           bool
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
	kind   agent.BlockKind
	text   agent.StreamedText
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

func (t *transcriptView) ToggleDetails() {
	first := t.content.FirstBlock()
	// #nosec G115 -- Transcript.Len is non-negative and cannot exceed the
	// addressable in-memory slice backing the transcript.
	end := first + headless.BlockID(t.content.Len())
	expand, hasTool := false, false
	for _, tracked := range t.toolViews {
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
		expand = !t.details
	}
	t.details = expand
	selectedChanged := false
	for _, tracked := range t.toolViews {
		if tracked.id < first || tracked.id >= end || !tracked.block.Expandable() {
			continue
		}
		before := tracked.block.Expanded()
		tracked.block.SetExpanded(t.details)
		if tracked.block.Expanded() == before {
			continue
		}
		selectedChanged = selectedChanged || t.focused && tracked.id == t.selected
		t.content.Changed(tracked.id)
	}
	if selectedChanged {
		t.revealSelected()
	}
	t.refreshSearch()
	t.announceSelection()
}

func (t *transcriptView) DetailsLabel() string {
	if t.details {
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
		pendingQuestions: make(map[string]trackedQuestion),
		runEntries:       make(map[string][]headless.BlockID),
		runLineages:      make(map[string]agent.RunLineage),
		keys:             transcriptKeys(),
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

func (t *transcriptView) Draw(frame headless.Frame) {
	width, _ := frame.Size()
	t.presentedBlocks.Stage(frame, transcriptBlockPresentation{
		epoch: t.contentEpoch, blocks: t.projectBlockPlacements(width),
	})
	t.view.Matches, t.view.Current = t.matches, t.current
	t.view.Draw(frame)
	if t.content.Len() == 0 && t.entrance != nil {
		t.entrance.Draw(frame.View)
	}
}

// SetEntrance installs a presentation-only projection that is consumed by the
// first transcript block or reset. It is not part of retained transcript state.
func (t *transcriptView) SetEntrance(entrance grid.Drawable) { t.entrance = entrance }

func (t *transcriptView) Handle(event input.Event) bool {
	if key, ok := event.(input.Key); ok && key.Down() {
		t.pointerGesture.cancel()
		if t.focused {
			if key.Code == input.Esc && t.selection.Active() {
				t.selection.Clear()
				return true
			}
			if _, handled := t.matcher.Handle(t.keys, key, t.Do); handled {
				return true
			}
		}
	}
	handled := t.view.Handle(event)
	mouse, ok := event.(input.Mouse)
	if !ok {
		return handled
	}
	if !handled {
		t.cancelPointerGesture(mouse)
		return false
	}
	t.handleMouse(mouse)
	return true
}

func (t *transcriptView) Focus(has bool) {
	if t.focused == has {
		return
	}
	t.focused = has
	if !has {
		t.matcher.Clear()
		t.pointerGesture.cancel()
	}
	if has {
		t.ensureSelection()
	}
	t.syncSelectedEntry()
	if t.onFocusChange != nil {
		t.onFocusChange(has)
	}
	t.announceSelection()
}

func (t *transcriptView) Focused() bool { return t.focused }

func (t *transcriptView) OnFocusChange(change func(bool)) { t.onFocusChange = change }

func (t *transcriptView) OnSelection(change func(transcriptSelection)) { t.onSelection = change }

func (t *transcriptView) OnCopy(copied func(string)) { t.onCopy = copied }

func (t *transcriptView) Keys() *keymap.Map { return t.keys }

func (t *transcriptView) action(event input.Event) keymap.Action {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return ""
	}
	action, _ := t.keys.Action(key.Chord())
	return action
}

func (t *transcriptView) Do(action keymap.Action) bool {
	switch action {
	case headless.SelectPrev:
		return t.moveSelection(-1)
	case headless.SelectNext:
		return t.moveSelection(1)
	case headless.SelectFirst:
		return t.selectEdge(false)
	case headless.SelectLast:
		return t.selectEdge(true)
	case headless.Collapse:
		return t.setSelectedExpanded(false)
	case headless.Expand:
		return t.setSelectedExpanded(true)
	case toggleDetails:
		return t.toggleSelected()
	case headless.Copy:
		return t.copySelected()
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

func (t *transcriptView) handleMouse(mouse input.Mouse) {
	if mouse.Button != input.ButtonLeft {
		t.cancelPointerGesture(mouse)
		return
	}
	switch mouse.Action {
	case input.MouseDown:
		point, _ := t.selection.Range()
		presented := t.presentedBlocks.Value()
		if presented.epoch != t.contentEpoch {
			// The transcript widget has already translated the press through its last
			// complete frame. Cancel the resulting selection when that frame belonged
			// to content Reset has replaced; BlockIDs restart from zero and must not
			// transfer a gesture to their new owners.
			t.selection.Clear()
			t.pointerGesture.cancel()
			return
		}
		id, offset, ok := presentedBlockAt(presented.blocks, point.Row)
		if !ok {
			t.pointerGesture.cancel()
			return
		}
		if !t.selectPointerEntry(id) {
			t.pointerGesture.cancel()
			return
		}
		t.pointerGesture.begin(id, offset == 0 && t.tool(id) != nil)
	case input.MouseDrag:
		t.pointerGesture.drag()
	case input.MouseUp:
		start, end := t.selection.Range()
		click, activate := t.pointerGesture.release(t.selected, t.hasSelected, start == end)
		if click {
			t.selection.Clear()
		}
		if activate {
			t.toggleSelected()
		} else if !click {
			t.copySelection()
		}
	}
}

func (t *transcriptView) cancelPointerGesture(mouse input.Mouse) {
	switch mouse.Action {
	case input.MouseDown, input.MouseUp, input.MouseDrag:
		t.pointerGesture.cancel()
	}
}

// projectBlockPlacements records stable block identities alongside the exact row
// layout being drawn. Semantic transcript content may change before the next frame;
// pointer input must still target what the last complete frame showed, not whatever
// now happens to occupy the same row number.
func (t *transcriptView) projectBlockPlacements(width int) []transcriptBlockPlacement {
	placements := make([]transcriptBlockPlacement, 0, t.content.Len())
	top := t.content.StartRow()
	first := t.content.FirstBlock()
	for index := range t.content.Len() {
		id := first + blockOffset(index)
		height := 0
		if width == t.content.Width() {
			_, height, _ = t.content.Extent(id)
		} else if entry := t.entries[id]; entry != nil && width > 0 {
			height = max(entry.Measure(width), 0)
		}
		placements = append(placements, transcriptBlockPlacement{blockID: id, top: top, height: height})
		top = layout.Sum(top, height)
	}
	return placements
}

func presentedBlockAt(placements []transcriptBlockPlacement, row int) (headless.BlockID, int, bool) {
	index := sort.Search(len(placements), func(index int) bool {
		return layout.Sum(placements[index].top, placements[index].height) > row
	})
	if index >= len(placements) || row < placements[index].top || placements[index].height <= 0 {
		return 0, 0, false
	}
	return placements[index].blockID, row - placements[index].top, true
}

func (t *transcriptView) ensureSelection() {
	first := t.content.FirstBlock()
	if t.hasSelected && t.selected >= first && t.selected < first+blockOffset(t.content.Len()) {
		return
	}
	if t.content.Len() == 0 {
		t.hasSelected = false
		return
	}
	t.selected = first + blockOffset(t.content.Len()-1)
	t.hasSelected = true
	t.revealSelected()
}

func (t *transcriptView) moveSelection(delta int) bool {
	if t.content.Len() == 0 || delta == 0 {
		return false
	}
	t.ensureSelection()
	first := t.content.FirstBlock()
	last := first + blockOffset(t.content.Len()-1)
	next := t.selected
	switch {
	case delta < 0 && next > first:
		next--
	case delta > 0 && next < last:
		next++
	default:
		return true
	}
	t.selectEntry(next, true)
	return true
}

func (t *transcriptView) selectEdge(last bool) bool {
	if t.content.Len() == 0 {
		return false
	}
	id := t.content.FirstBlock()
	if last {
		id += blockOffset(t.content.Len() - 1)
	}
	t.selectEntry(id, true)
	return true
}

func (t *transcriptView) selectEntry(id headless.BlockID, reveal bool) {
	t.setSelectedEntry(id, reveal, true)
}

func (t *transcriptView) selectPointerEntry(id headless.BlockID) bool {
	if _, ok := t.entries[id]; !ok {
		return false
	}
	t.setSelectedEntry(id, false, false)
	return true
}

func (t *transcriptView) setSelectedEntry(id headless.BlockID, reveal, clearTextSelection bool) {
	if _, ok := t.entries[id]; !ok {
		return
	}
	t.selected, t.hasSelected = id, true
	if clearTextSelection {
		t.selection.Clear()
	}
	t.syncSelectedEntry()
	if reveal {
		t.revealSelected()
	}
	t.announceSelection()
}

func (t *transcriptView) syncSelectedEntry() {
	for id, entry := range t.entries {
		entry.selected = t.hasSelected && id == t.selected
		entry.focused = entry.selected && t.focused
	}
}

func (t *transcriptView) revealSelected() {
	if !t.hasSelected {
		return
	}
	if top, height, ok := t.content.Extent(t.selected); ok {
		start := t.content.StartRow()
		t.scroll.RevealRange(top-start, top-start+height-1)
	}
}

func (t *transcriptView) tool(id headless.BlockID) toolDisclosure {
	for _, tracked := range t.toolViews {
		if tracked.id == id {
			return tracked.block
		}
	}
	return nil
}

func (t *transcriptView) toggleSelected() bool {
	return t.mutateSelectedDisclosure(func(tool toolDisclosure) {
		tool.ToggleExpanded()
	})
}

func (t *transcriptView) setSelectedExpanded(expanded bool) bool {
	return t.mutateSelectedDisclosure(func(tool toolDisclosure) {
		tool.SetExpanded(expanded)
	})
}

// mutateSelectedDisclosure owns the layout invariant for keyboard-operated tool
// details. Both expansion and collapse can move the selected entry relative to
// the viewport, so every actual height change remeasures and reveals the same
// stable block identity before another command can target it.
func (t *transcriptView) mutateSelectedDisclosure(mutate func(toolDisclosure)) bool {
	tool := t.tool(t.selected)
	if !t.hasSelected || tool == nil || !tool.Expandable() {
		return true
	}
	before := tool.Expanded()
	mutate(tool)
	if tool.Expanded() != before {
		t.content.Changed(t.selected)
		t.revealSelected()
		t.refreshSearch()
	}
	t.announceSelection()
	return true
}

func (t *transcriptView) copySelected() bool {
	if !t.hasSelected {
		return true
	}
	top, height, ok := t.content.Extent(t.selected)
	if !ok {
		return true
	}
	t.copy(copyableRowsText(t.content.Rows(top, height)))
	return true
}

func (t *transcriptView) copySelection() {
	t.copy(t.selection.Text(&t.content))
}

func (t *transcriptView) copy(value string) {
	if value == "" {
		return
	}
	if t.clipboard == nil || !t.clipboard.Copy(value) {
		return
	}
	if t.onCopy != nil {
		t.onCopy(value)
	}
}

func (t *transcriptView) announceSelection() {
	if t.onSelection == nil {
		return
	}
	_, readable := t.readerTargetForSelected()
	selection := transcriptSelection{Present: t.hasSelected, Readable: readable}
	if tool := t.tool(t.selected); tool != nil && tool.Expandable() {
		selection.Expandable = true
		selection.Expanded = tool.Expanded()
	}
	t.onSelection(selection)
}

func (t *transcriptView) selectedReaderTarget() (readerTarget, bool) {
	if !t.hasSelected {
		t.ensureSelection()
	}
	return t.readerTargetForSelected()
}

func (t *transcriptView) readerTargetForSelected() (readerTarget, bool) {
	if !t.hasSelected {
		return readerTarget{}, false
	}
	entry := t.entries[t.selected]
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
	width := max(t.content.Width()-transcriptEntryInset, 40)
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

func (t *transcriptView) Follow() { t.scroll.ToBottom() }

func (t *transcriptView) Scroll(action keymap.Action) bool { return t.scroll.Do(action) }

func (t *transcriptView) Close() {
	if t != nil && t.search != nil {
		t.search.Close()
	}
}

func (t *transcriptView) Apply(event agent.Event, registry *extensions.Registry) error {
	return t.apply("", event, registry)
}

func (t *transcriptView) ApplyRunEvent(envelope agent.RunEvent, registry *extensions.Registry) error {
	if started, ok := envelope.Event.(agent.SegmentStarted); ok {
		t.runLineages[started.Run.ID] = started.Run.Lineage
	}
	return t.apply(envelope.RunID, envelope.Event, registry)
}

func (t *transcriptView) apply(runID string, event agent.Event, registry *extensions.Registry) error {
	switch e := event.(type) {
	case agent.BlockStarted:
		if e.Block.Kind == agent.BlockAssistant || e.Block.Kind == agent.BlockReasoning {
			return t.begin(e.Block)
		}
		if e.Block.Kind == agent.BlockTool {
			return t.beginTool(e.Block, registry)
		}
		t.sealToolGroup()
	case agent.BlockDelta:
		key := transcriptBlockKey(runID, e.BlockID)
		if _, live := t.tools[key]; live {
			return t.deltaTool(key, e)
		}
		return t.delta(key, e)
	case agent.ToolArgumentsDelta, agent.RunProgress:
		// Tool arguments are provisional JSON and progress belongs in the status
		// chrome. Neither creates an authoritative transcript block.
	case agent.CustomEvent:
		return t.appendCustom(runID, e, registry)
	case agent.BlockCompleted:
		return t.complete(e.Block, registry)
	case agent.RunFinished:
		if strings.TrimSpace(runID) == "" {
			t.settleLive(e.Outcome)
		} else {
			t.settleRun(runID, e.Outcome)
		}
	case agent.RunInterrupted:
		t.sealToolGroup()
	}
	return nil
}

func (t *transcriptView) appendCustom(runID string, event agent.CustomEvent, registry *extensions.Registry) error {
	for _, presenter := range registry.Values(CustomEventPresenters) {
		if presenter.Name != event.Name {
			continue
		}
		rendered, err := presentCustomSafely(presenter, BlockPresentation{
			Theme: t.theme, Glyphs: t.glyphs, Look: t.look, Syntax: t.syntax,
			Tools: registry.Values(ToolPresenters), Speaker: "runtime", Image: t.presentImage,
		}, event)
		if err != nil {
			return err
		}
		t.sealToolGroup()
		for _, block := range rendered {
			id := t.append(block)
			t.trackRunEntry(runID, id)
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

func (t *transcriptView) begin(block agent.Block) error {
	key := transcriptBlockKey(block.RunID, block.ID)
	if _, exists := t.textStreams[key]; exists {
		return fmt.Errorf("terminal transcript: text block %s started twice", block.ID)
	}
	if _, exists := t.tools[key]; exists {
		return fmt.Errorf("terminal transcript: block %s is already a live tool", block.ID)
	}
	t.sealToolGroup()
	speaker := t.speakerFor(block)
	live := &liveText{
		runID: block.RunID, kind: block.Kind, text: agent.NewStreamedText(block.Text),
		block: &markdownBlock{theme: t.theme, speaker: speaker},
	}
	live.stream.SetLook(t.lookFor(block.Kind))
	live.id = t.place(live.block, false)
	t.trackRunEntry(block.RunID, live.id)
	t.textStreams[key] = live
	if block.Text != "" {
		t.updateLiveText(live, agent.TextMutation{Text: block.Text})
	}
	return nil
}

func (t *transcriptView) delta(key string, delta agent.BlockDelta) error {
	live, ok := t.textStreams[key]
	if !ok {
		return fmt.Errorf("terminal transcript: delta for inactive text block %s", delta.BlockID)
	}
	if live.kind != agent.BlockAssistant && delta.ContentIndex != nil {
		return fmt.Errorf("terminal transcript: %s block %s has a content index", live.kind, delta.BlockID)
	}
	mutation, err := live.text.Apply(delta)
	if err != nil {
		return fmt.Errorf("terminal transcript: stream text block %s: %w", delta.BlockID, err)
	}
	t.updateLiveText(live, mutation)
	return nil
}

func (t *transcriptView) updateLiveText(live *liveText, mutation agent.TextMutation) {
	if mutation.Replace {
		live.stream.Reset()
		live.stable = nil
	}
	live.stable = append(live.stable, live.stream.Feed(mutation.Text)...)
	blocks := slices.Clone(live.stable)
	blocks = append(blocks, live.stream.Open()...)
	live.block.doc.SetBlocks(blocks)
	t.content.Changed(live.id)
	t.refreshSearch()
}

func (t *transcriptView) deltaTool(key string, delta agent.BlockDelta) error {
	live, ok := t.tools[key]
	if !ok {
		return fmt.Errorf("terminal transcript: delta for inactive tool block %s", delta.BlockID)
	}
	if delta.ContentIndex != nil {
		return fmt.Errorf("terminal transcript: tool block %s has a content index", delta.BlockID)
	}
	for _, tracked := range live.blocks {
		tracked.block.AppendOutput(delta.Text)
		t.content.Changed(tracked.id)
	}
	t.refreshSearch()
	t.announceSelection()
	return nil
}

func (t *transcriptView) complete(block agent.Block, registry *extensions.Registry) error {
	key := transcriptBlockKey(block.RunID, block.ID)
	if _, live := t.textStreams[key]; live {
		return t.completeStream(block)
	}
	if block.Kind == agent.BlockTool && t.completeLiveTool(block) {
		return nil
	}
	return t.appendCompleted(block, registry)
}

func (t *transcriptView) completeStream(block agent.Block) error {
	key := transcriptBlockKey(block.RunID, block.ID)
	live, ok := t.textStreams[key]
	if !ok {
		return fmt.Errorf("terminal transcript: completion for inactive text block %s", block.ID)
	}
	// The completed value is authoritative. Re-rendering it once also repairs a
	// transport that intentionally replaced an earlier provisional tail.
	live.block.doc.SetBlocks(markdown.Render(block.Text, t.lookFor(block.Kind)))
	t.content.Changed(live.id)
	t.content.Finish(live.id)
	live.stream.Reset()
	delete(t.textStreams, key)
	for _, image := range block.Images {
		id := t.append(t.presentImage(image))
		t.trackRunEntry(block.RunID, id)
	}
	t.refreshSearch()
	return nil
}

func (t *transcriptView) completeLiveTool(block agent.Block) bool {
	key := transcriptBlockKey(block.RunID, block.ID)
	live, ok := t.tools[key]
	if !ok {
		return false
	}
	selectedCollapsed := false
	for _, tracked := range live.blocks {
		selectedCollapsed = t.mutateTrackedTool(tracked, func(tool mutableToolBlock) { tool.Update(block) }) || selectedCollapsed
	}
	if selectedCollapsed {
		t.revealSelected()
	}
	if live.group != nil {
		t.finishToolGroupIfReady(live.group)
	} else {
		for _, id := range live.ids {
			t.content.Finish(id)
		}
	}
	delete(t.tools, key)
	if len(live.blocks) == 0 {
		return false
	}
	t.refreshSearch()
	t.announceSelection()
	return true
}

func (t *transcriptView) settleLive(outcome agent.Outcome) {
	for id, live := range t.textStreams {
		t.content.Finish(live.id)
		live.stream.Reset()
		delete(t.textStreams, id)
	}
	toolStatus := agent.ToolError
	if outcome.Status == agent.OutcomeCanceled {
		toolStatus = agent.ToolCanceled
	}
	selectedCollapsed := false
	for id, live := range t.tools {
		for _, tracked := range live.blocks {
			selectedCollapsed = t.mutateTrackedTool(tracked, func(tool mutableToolBlock) { tool.Finish(toolStatus) }) || selectedCollapsed
		}
		if live.group != nil {
			t.finishToolGroupIfReady(live.group)
		} else {
			for _, blockID := range live.ids {
				t.content.Finish(blockID)
			}
		}
		delete(t.tools, id)
	}
	t.finishPendingQuestions("")
	if selectedCollapsed {
		t.revealSelected()
	}
	t.sealToolGroup()
	t.refreshSearch()
	t.announceSelection()
}

func (t *transcriptView) settleRun(runID string, outcome agent.Outcome) {
	for id, live := range t.textStreams {
		if live.runID != runID {
			continue
		}
		t.content.Finish(live.id)
		live.stream.Reset()
		delete(t.textStreams, id)
	}
	toolStatus := agent.ToolError
	if outcome.Status == agent.OutcomeCanceled {
		toolStatus = agent.ToolCanceled
	}
	selectedCollapsed := false
	for id, live := range t.tools {
		if live.runID != runID {
			continue
		}
		for _, tracked := range live.blocks {
			selectedCollapsed = t.mutateTrackedTool(tracked, func(tool mutableToolBlock) { tool.Finish(toolStatus) }) || selectedCollapsed
		}
		if live.group != nil {
			t.finishToolGroupIfReady(live.group)
		} else {
			for _, blockID := range live.ids {
				t.content.Finish(blockID)
			}
		}
		delete(t.tools, id)
	}
	t.finishPendingQuestions(runID)
	if selectedCollapsed {
		t.revealSelected()
	}
	if t.activeToolGroup != nil && t.activeToolGroup.runID == runID {
		t.sealToolGroup()
	}
	t.refreshSearch()
	t.announceSelection()
}

func (t *transcriptView) mutateTrackedTool(tracked trackedTool, mutate func(mutableToolBlock)) bool {
	before := tracked.block.Expanded()
	mutate(tracked.block)
	t.content.Changed(tracked.id)
	return t.focused && tracked.id == t.selected && before && !tracked.block.Expanded()
}

func (t *transcriptView) appendCompleted(block agent.Block, registry *extensions.Registry) error {
	rendered, err := t.present(block, registry)
	if err != nil {
		return err
	}
	if block.Kind == agent.BlockTool {
		if tool, grouped := groupedTool(rendered); grouped {
			t.addGroupedTool(block.RunID, tool)
			t.refreshSearch()
			return nil
		}
	}
	t.sealToolGroup()
	for _, item := range rendered {
		mutable, isMutable := item.(mutableToolBlock)
		if isMutable {
			mutable.SetExpanded(t.details)
		}
		question, isPendingQuestion := item.(*questionBlock)
		isPendingQuestion = isPendingQuestion && !question.answered()
		key := ""
		if isPendingQuestion {
			key = transcriptBlockKey(block.RunID, block.ID)
			if _, exists := t.pendingQuestions[key]; exists {
				return fmt.Errorf("terminal transcript: question block %s completed twice", block.ID)
			}
		}
		id := t.place(item, !isPendingQuestion)
		t.trackRunEntry(block.RunID, id)
		if isPendingQuestion {
			t.pendingQuestions[key] = trackedQuestion{runID: block.RunID, id: id, block: question}
		}
		if isMutable {
			t.toolViews = append(t.toolViews, trackedToolView{id: id, block: mutable})
		}
		if block.Kind == agent.BlockUser {
			t.sticky.Add(id)
		}
	}
	t.refreshSearch()
	return nil
}

func (t *transcriptView) beginTool(block agent.Block, registry *extensions.Registry) error {
	key := transcriptBlockKey(block.RunID, block.ID)
	if _, exists := t.tools[key]; exists {
		return fmt.Errorf("terminal transcript: tool block %s started twice", block.ID)
	}
	if _, exists := t.textStreams[key]; exists {
		return fmt.Errorf("terminal transcript: block %s is already a live text block", block.ID)
	}
	rendered, err := t.present(block, registry)
	if err != nil {
		return err
	}
	if tool, grouped := groupedTool(rendered); grouped {
		group := t.addGroupedTool(block.RunID, tool)
		tracked := trackedTool{id: group.id, block: tool}
		t.tools[key] = liveTool{runID: block.RunID, blocks: []trackedTool{tracked}, group: group}
		t.refreshSearch()
		return nil
	}
	t.sealToolGroup()
	live := liveTool{runID: block.RunID}
	for _, item := range rendered {
		mutable, isMutable := item.(mutableToolBlock)
		if isMutable {
			mutable.SetExpanded(t.details)
		}
		id := t.place(item, false)
		t.trackRunEntry(block.RunID, id)
		live.ids = append(live.ids, id)
		if isMutable {
			tracked := trackedTool{id: id, block: mutable}
			live.blocks = append(live.blocks, tracked)
			t.toolViews = append(t.toolViews, trackedToolView{id: id, block: mutable})
		}
	}
	t.tools[key] = live
	t.refreshSearch()
	return nil
}

func groupedTool(rendered []headless.Block) (*toolBlock, bool) {
	if len(rendered) != 1 {
		return nil, false
	}
	tool, ok := rendered[0].(*toolBlock)
	return tool, ok && groupableTool(tool.call)
}

func (t *transcriptView) addGroupedTool(runID string, tool *toolBlock) *trackedToolGroup {
	group := t.activeToolGroup
	if group == nil || group.runID != runID {
		t.sealToolGroup()
		block := newToolGroupBlock(t.theme, t.glyphs, t.details)
		block.Add(tool)
		group = &trackedToolGroup{runID: runID, block: block}
		group.id = t.place(block, false)
		t.trackRunEntry(runID, group.id)
		t.toolViews = append(t.toolViews, trackedToolView{id: group.id, block: block})
		t.activeToolGroup = group
		return group
	}
	group.block.Add(tool)
	t.content.Changed(group.id)
	return group
}

func (t *transcriptView) sealToolGroup() {
	group := t.activeToolGroup
	if group == nil {
		return
	}
	group.block.Seal()
	t.content.Changed(group.id)
	t.activeToolGroup = nil
	t.finishToolGroupIfReady(group)
}

func (t *transcriptView) finishToolGroupIfReady(group *trackedToolGroup) {
	if group != nil && group.block.ReadyToFinish() {
		t.content.Finish(group.id)
	}
}

// SealToolGroups closes the trailing adjacency window after a cold snapshot.
// A live event stream closes it naturally on the next semantic boundary.
func (t *transcriptView) SealToolGroups() { t.sealToolGroup() }

func (t *transcriptView) present(block agent.Block, registry *extensions.Registry) ([]headless.Block, error) {
	for _, presenter := range registry.Values(BlockPresenters) {
		if presenter.Kind == block.Kind {
			return presentSafely(presenter, BlockPresentation{
				Theme: t.theme, Glyphs: t.glyphs, Look: t.look, Syntax: t.syntax,
				Tools: registry.Values(ToolPresenters), Speaker: t.speakerFor(block), Image: t.presentImage,
			}, block)
		}
	}
	return nil, fmt.Errorf("terminal transcript: no presenter for block kind %q", block.Kind)
}

func (t *transcriptView) presentImage(image agent.InlineImage) headless.Block {
	if t.images != nil {
		return t.images.Present(t.theme, image)
	}
	return fallbackInlineImage(t.theme, image)
}

func presentSafely(presenter BlockPresenter, presentation BlockPresentation, block agent.Block) (rendered []headless.Block, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("terminal transcript: presenter for %q panicked: %v", presenter.Kind, recovered)
		}
	}()
	return presenter.Present(presentation, block), nil
}

func (t *transcriptView) Append(block headless.Block) {
	t.sealToolGroup()
	t.append(block)
}

func (t *transcriptView) append(block headless.Block) headless.BlockID {
	id := t.place(block, true)
	t.refreshSearch()
	return id
}

func (t *transcriptView) place(block headless.Block, finished bool) headless.BlockID {
	t.entrance = nil
	entry := newTranscriptEntry(t.theme, t.glyphs, block)
	id := t.content.Append(entry)
	t.entries[id] = entry
	if finished {
		t.content.Finish(id)
	}
	return id
}

type discardedOutput struct{}

func (discardedOutput) Print(grid.Drawable) {}

func (t *transcriptView) DiscardExcess() {
	if t.content.Width() <= 0 {
		return
	}
	finished := 0
	for i := range t.content.Len() {
		id := t.content.FirstBlock() + headless.BlockID(i)
		if !t.content.Finished(id) {
			break
		}
		finished++
	}
	if excess := finished - t.retain; excess > 0 {
		t.view.Commit(discardedOutput{}, excess)
	}
	first := t.content.FirstBlock()
	t.toolViews = slices.DeleteFunc(t.toolViews, func(item trackedToolView) bool { return item.id < first })
	for id := range t.entries {
		if id < first {
			delete(t.entries, id)
		}
	}
	for key, question := range t.pendingQuestions {
		if question.id < first {
			delete(t.pendingQuestions, key)
		}
	}
	for runID, ids := range t.runEntries {
		ids = slices.DeleteFunc(ids, func(id headless.BlockID) bool { return id < first })
		if len(ids) == 0 {
			delete(t.runEntries, runID)
		} else {
			t.runEntries[runID] = ids
		}
	}
	if t.hasSelected && t.selected < first {
		t.hasSelected = false
		t.ensureSelection()
		t.syncSelectedEntry()
		t.announceSelection()
	}
	t.refreshSearch()
}

func (t *transcriptView) Reset() {
	t.entrance = nil
	t.contentEpoch++
	t.content = headless.Transcript{}
	t.scroll = headless.Scroll{}
	t.scroll.Wheel(t.wheel)
	t.scroll.ToBottom()
	t.selection = headless.Selection{}
	t.sticky = headless.Sticky{MinHeight: 1, Gap: 1}
	t.view.Content, t.view.Scroll = &t.content, &t.scroll
	t.view.Selection, t.view.Sticky = &t.selection, &t.sticky
	for _, live := range t.textStreams {
		live.stream.Reset()
	}
	clear(t.textStreams)
	t.query, t.matches, t.current, t.announceSearch = "", nil, -1, false
	t.searchCursor = transcriptSearchCursor{}
	clear(t.tools)
	clear(t.pendingQuestions)
	clear(t.entries)
	clear(t.runEntries)
	clear(t.runLineages)
	t.activeToolGroup = nil
	t.hasSelected = false
	t.pointerGesture.cancel()
	t.toolViews = nil
	t.search.Submit(&t.content, "", false)
}

func (t *transcriptView) SetRuns(runs []agent.Run) {
	clear(t.runLineages)
	for _, run := range runs {
		t.runLineages[run.ID] = run.Lineage
	}
}

func (t *transcriptView) speakerFor(block agent.Block) string {
	lineage, known := t.runLineages[block.RunID]
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

func (t *transcriptView) trackRunEntry(runID string, id headless.BlockID) {
	if strings.TrimSpace(runID) == "" {
		return
	}
	t.runEntries[runID] = append(t.runEntries[runID], id)
}

func (t *transcriptView) JumpToRun(runID string) bool {
	first := t.content.FirstBlock()
	last := first + blockOffset(t.content.Len())
	for _, id := range t.runEntries[runID] {
		if id < first || id >= last {
			continue
		}
		t.selectEntry(id, true)
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

func (t *transcriptView) Find(query string) {
	t.query = strings.TrimSpace(query)
	t.announceSearch = t.query != ""
	t.matches, t.current = nil, -1
	t.searchCursor = transcriptSearchCursor{}
	t.search.Submit(&t.content, t.query, false)
}

func (t *transcriptView) refreshSearch() {
	if t.query != "" {
		t.search.Submit(&t.content, t.query, false)
	}
}

func (t *transcriptView) SearchResults() <-chan headless.Result { return t.search.Results() }

func (t *transcriptView) AcceptSearch(result headless.Result) (accepted, announce bool) {
	if result.Query != t.query {
		return false, false
	}
	next := t.searchMatchIndex(result.Matches)
	t.matches = result.Matches
	if len(t.matches) > 0 {
		t.current = next
		t.rememberSearchCursor()
	} else {
		t.current = -1
		t.searchCursor = transcriptSearchCursor{}
	}
	announce, t.announceSearch = t.announceSearch, false
	return true, announce
}

func (t *transcriptView) StepMatch(delta int) bool {
	if len(t.matches) == 0 {
		return false
	}
	t.current = (t.current + delta) % len(t.matches)
	if t.current < 0 {
		t.current += len(t.matches)
	}
	t.rememberSearchCursor()
	return true
}

func (t *transcriptView) searchMatchIndex(matches []headless.Match) int {
	if len(matches) == 0 || !t.searchCursor.present {
		return 0
	}
	best := -1
	var bestRowDistance, bestColumnDistance uint
	for index, match := range matches {
		id, offset, ok := t.content.At(match.Row)
		if !ok || id != t.searchCursor.blockID {
			continue
		}
		col := 0
		if len(match.Spans) > 0 {
			col = match.Spans[0].Col
		}
		rowDistance := unsignedDistance(offset, t.searchCursor.rowOffset)
		columnDistance := unsignedDistance(col, t.searchCursor.column)
		if best < 0 || rowDistance < bestRowDistance ||
			(rowDistance == bestRowDistance && columnDistance < bestColumnDistance) {
			best, bestRowDistance, bestColumnDistance = index, rowDistance, columnDistance
		}
	}
	if best >= 0 {
		return best
	}
	return min(t.searchCursor.index, len(matches)-1)
}

func (t *transcriptView) rememberSearchCursor() {
	if t.current < 0 || t.current >= len(t.matches) {
		t.searchCursor = transcriptSearchCursor{}
		return
	}
	match := t.matches[t.current]
	id, offset, ok := t.content.At(match.Row)
	if !ok {
		t.searchCursor = transcriptSearchCursor{}
		return
	}
	col := 0
	if len(match.Spans) > 0 {
		col = match.Spans[0].Col
	}
	t.searchCursor = transcriptSearchCursor{
		blockID: id, rowOffset: offset, column: col, index: t.current, present: true,
	}
}

func unsignedDistance(left, right int) uint {
	if left < right {
		left, right = right, left
	}
	return uint(left) - uint(right)
}

func (t *transcriptView) lookFor(kind agent.BlockKind) markdown.Look {
	look := t.look
	if kind == agent.BlockReasoning {
		look.Text, look.Strong, look.Code = t.theme.Muted, t.theme.Subtle, t.theme.Info
	}
	return look
}
