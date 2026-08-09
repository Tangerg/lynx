package session

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
	"github.com/Tangerg/oolong/highlight"
	"github.com/Tangerg/oolong/markdown"

	"github.com/Tangerg/lynx/app/cli/internal/client"
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
	tools          map[string]liveTool
	toolViews      []trackedTool

	streamID string
	stream   markdown.Stream
	stable   []markdown.Block
	open     *markdownMessage
	openID   headless.BlockID
}

type liveTool struct {
	ids    []headless.BlockID
	blocks []trackedTool
}

type trackedTool struct {
	id    headless.BlockID
	block mutableToolBlock
}

func (c *conversationView) ToggleDetails() {
	c.details = !c.details
	first := c.content.FirstBlock()
	// #nosec G115 -- Transcript.Len is non-negative and cannot exceed the
	// addressable in-memory slice backing the transcript.
	end := first + headless.BlockID(c.content.Len())
	for _, tracked := range c.toolViews {
		tracked.block.SetExpanded(c.details)
		if tracked.id >= first && tracked.id < end {
			c.content.Changed(tracked.id)
		}
	}
	c.scroll.ToBottom()
	c.refreshSearch()
}

func (c *conversationView) DetailsLabel() string {
	if c.details {
		return "tool details expanded"
	}
	return "tool details collapsed"
}

func newConversationView(theme kit.Theme, glyphs kit.Glyphs, wheel input.Wheel, syntax highlight.Style, retain int, details bool) *conversationView {
	c := &conversationView{
		theme: theme, glyphs: glyphs, wheel: wheel,
		look: markdownLook(theme, glyphs, syntax), syntax: syntax,
		search: headless.NewSearch(), current: -1, retain: max(retain, 4), details: details, tools: make(map[string]liveTool),
	}
	c.scroll.Wheel(wheel)
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

func (c *conversationView) Handle(event input.Event) bool { return c.view.Handle(event) }

func (c *conversationView) Close() {
	if c != nil && c.search != nil {
		c.search.Close()
	}
}

func (c *conversationView) Apply(event client.Event, registry *extensions.Registry) error {
	switch e := event.(type) {
	case client.BlockStarted:
		if e.Block.Kind == client.BlockAssistant || e.Block.Kind == client.BlockReasoning {
			return c.begin(e.Block)
		}
		if e.Block.Kind == client.BlockTool {
			return c.beginTool(e.Block, registry)
		}
	case client.BlockDelta:
		return c.delta(e.BlockID, e.Text)
	case client.BlockCompleted:
		return c.complete(e.Block, registry)
	}
	return nil
}

func (c *conversationView) begin(block client.Block) error {
	if c.streamID != "" {
		return fmt.Errorf("terminal transcript: block %s started while %s is still streaming", block.ID, c.streamID)
	}
	speaker := "lyra"
	if block.Kind == client.BlockReasoning {
		speaker = "thinking"
	}
	c.streamID = block.ID
	c.stream.Reset()
	c.stream.SetLook(c.lookFor(block.Kind))
	c.stable = c.stable[:0]
	c.open = &markdownMessage{theme: c.theme, speaker: speaker}
	c.openID = c.content.Append(c.open)
	if block.Text != "" {
		return c.delta(block.ID, block.Text)
	}
	return nil
}

func (c *conversationView) delta(id, chunk string) error {
	if id != c.streamID || c.open == nil {
		return fmt.Errorf("terminal transcript: delta for inactive block %s", id)
	}
	c.stable = append(c.stable, c.stream.Feed(chunk)...)
	blocks := append([]markdown.Block(nil), c.stable...)
	blocks = append(blocks, c.stream.Open()...)
	c.open.doc.SetBlocks(blocks)
	c.content.Changed(c.openID)
	c.scroll.ToBottom()
	c.refreshSearch()
	return nil
}

func (c *conversationView) complete(block client.Block, registry *extensions.Registry) error {
	if block.ID == c.streamID {
		// The completed value is authoritative. Re-rendering it once also repairs a
		// transport that intentionally replaced an earlier provisional tail.
		c.open.doc.SetBlocks(markdown.Render(block.Text, c.lookFor(block.Kind)))
		c.content.Changed(c.openID)
		c.content.Finish(c.openID)
		c.streamID, c.open = "", nil
		c.stream.Reset()
		clear(c.stable)
		c.stable = c.stable[:0]
		c.scroll.ToBottom()
		c.refreshSearch()
		return nil
	}
	if c.streamID != "" {
		return fmt.Errorf("terminal transcript: block %s completed while %s is streaming", block.ID, c.streamID)
	}
	if block.Kind == client.BlockTool {
		if live, ok := c.tools[block.ID]; ok {
			if len(live.blocks) > 0 {
				for _, tracked := range live.blocks {
					tracked.block.Update(block)
					tracked.block.SetExpanded(c.details)
					c.content.Changed(tracked.id)
				}
				for _, id := range live.ids {
					c.content.Finish(id)
				}
				delete(c.tools, block.ID)
				c.scroll.ToBottom()
				c.refreshSearch()
				return nil
			}
			for _, id := range live.ids {
				c.content.Finish(id)
			}
			delete(c.tools, block.ID)
		}
	}

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
		if block.Kind == client.BlockUser {
			c.sticky.Add(id)
		}
	}
	return nil
}

func (c *conversationView) beginTool(block client.Block, registry *extensions.Registry) error {
	if _, exists := c.tools[block.ID]; exists {
		return fmt.Errorf("terminal transcript: tool block %s started twice", block.ID)
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
		id := c.content.Append(item)
		live.ids = append(live.ids, id)
		if isMutable {
			tracked := trackedTool{id: id, block: mutable}
			live.blocks = append(live.blocks, tracked)
			c.toolViews = append(c.toolViews, tracked)
		}
	}
	c.tools[block.ID] = live
	c.scroll.ToBottom()
	c.refreshSearch()
	return nil
}

func (c *conversationView) present(block client.Block, registry *extensions.Registry) ([]headless.Block, error) {
	for _, presenter := range extensions.Values(registry, BlockPresenters) {
		if presenter.Kind == block.Kind {
			return presentSafely(presenter, Presentation{
				Theme: c.theme, Glyphs: c.glyphs, Look: c.look, Syntax: c.syntax,
			}, block)
		}
	}
	return nil, fmt.Errorf("terminal transcript: no presenter for block kind %q", block.Kind)
}

func presentSafely(presenter BlockPresenter, presentation Presentation, block client.Block) (rendered []headless.Block, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("terminal transcript: presenter for %q panicked: %v", presenter.Kind, recovered)
		}
	}()
	return presenter.Present(presentation, block), nil
}

func (c *conversationView) Append(block headless.Block) { c.append(block) }

func (c *conversationView) append(block headless.Block) headless.BlockID {
	id := c.content.Append(block)
	c.content.Finish(id)
	c.scroll.ToBottom()
	c.refreshSearch()
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
	c.scroll.ToBottom()
	c.refreshSearch()
}

func (c *conversationView) Reset() {
	c.content = headless.Transcript{}
	c.scroll = headless.Scroll{}
	c.scroll.Wheel(c.wheel)
	c.selection = headless.Selection{}
	c.sticky = headless.Sticky{MinHeight: 1, Gap: 1}
	c.view.Content, c.view.Scroll = &c.content, &c.scroll
	c.view.Selection, c.view.Sticky = &c.selection, &c.sticky
	c.stream.Reset()
	c.streamID, c.open = "", nil
	clear(c.stable)
	c.stable = c.stable[:0]
	c.query, c.matches, c.current, c.announceSearch = "", nil, -1, false
	clear(c.tools)
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

func (c *conversationView) lookFor(kind client.BlockKind) markdown.Look {
	look := c.look
	if kind == client.BlockReasoning {
		look.Text, look.Strong, look.Code = c.theme.Muted, c.theme.Subtle, c.theme.Info
	}
	return look
}

type markdownMessage struct {
	theme   kit.Theme
	speaker string
	doc     markdown.Doc
}

func (m *markdownMessage) Measure(width int) int {
	if m == nil {
		return 0
	}
	return layout.Sum(1, m.doc.Measure(max(width-2, 1)), 1)
}

func (m *markdownMessage) Draw(view grid.View) {
	if m == nil {
		return
	}
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	view.Text(0, 0, m.speaker, m.theme.Muted)
	m.doc.Draw(view.Sub(grid.Rect(2, 1, max(width-2, 0), max(height-2, 0))))
}

func (m *markdownMessage) Rows(width int) []text.Row {
	if m == nil {
		return nil
	}
	rows := []text.Row{{Text: m.speaker}}
	for _, row := range m.doc.Rows(max(width-2, 1)) {
		row.Offset += 2
		rows = append(rows, row)
	}
	return append(rows, text.Row{})
}

func markdownLook(theme kit.Theme, glyphs kit.Glyphs, style highlight.Style) markdown.Look {
	return markdown.Look{
		Text: theme.Text, Headings: []grid.Style{theme.Heading, theme.Strong},
		Strong: theme.Strong, Emphasis: grid.Style{Attr: grid.Italic},
		Struck: theme.Muted, Code: theme.Info, Block: theme.Sunken,
		Link: theme.Accent, Quote: theme.Muted, Rail: theme.Subtle,
		Marker: theme.Accent, Rule: theme.Divider, Highlight: highlight.Of(style),
		Glyphs: markdown.Glyphs{
			Bullet: glyphs.Bullet, Bar: glyphs.Vertical, Divider: glyphs.Horizontal,
			Checked: glyphs.Taken, Unchecked: glyphs.Free,
		},
	}
}

func presentError(theme kit.Theme, message string) headless.Block {
	danger := theme
	danger.Text = theme.Danger
	return kit.Message{Theme: danger, Speaker: "runtime", Body: message}
}
