package terminal

import (
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
	"github.com/Tangerg/oolong/highlight"
)

const (
	readerFind     keymap.Action = "find"
	readerNext     keymap.Action = "next match"
	readerPrevious keymap.Action = "previous match"
	readerCopy     keymap.Action = "copy"
	readerClose    keymap.Action = "close"
)

type readerDocument struct {
	Title    string
	Detail   string
	Sections []ToolSection
}

type readerDocumentSource interface {
	Observe(func(readerDocument)) func()
}

type readerTarget struct {
	document readerDocument
	source   readerDocumentSource
}

type readerSelectionGesture struct {
	active bool
}

func (r *readerSelectionGesture) begin() { r.active = true }

func (r *readerSelectionGesture) release() bool {
	owned := r.active
	r.cancel()
	return owned
}

func (r *readerSelectionGesture) cancel() { r.active = false }

// readerPane owns one modal reading session: full semantic content, search,
// selection, scrolling, and an optional live tool subscription.
type readerPane struct {
	theme     kit.Theme
	glyphs    kit.Glyphs
	syntax    highlight.Renderer
	wheel     input.Wheel
	clipboard headless.Clipboard
	keys      *keymap.Map

	content          headless.Transcript
	scroll           headless.Scroll
	selection        headless.Selection
	view             kit.Transcript
	selectionGesture readerSelectionGesture
	search           *headless.Search
	matches          []headless.Match
	current          int
	query            string
	problem          string
	title            string
	detail           string

	dismiss         func()
	openSearch      func()
	onCopied        func()
	releaseSource   func()
	observingSource bool
}

func newReaderPane(theme kit.Theme, glyphs kit.Glyphs, syntax highlight.Renderer, wheel input.Wheel, clipboard headless.Clipboard) *readerPane {
	keys := headless.DefaultScrollKeys()
	keys.Bind(readerFind, input.Ctrl.Rune('f'))
	keys.Bind(readerNext, input.Chord{Code: input.F3})
	keys.Bind(readerPrevious, input.Shift.With(input.F3))
	keys.Bind(readerCopy, input.Alt.Rune('c'))
	keys.Bind(readerClose, input.Chord{Code: input.Esc})
	r := &readerPane{
		theme: theme, glyphs: glyphs, syntax: syntax, wheel: wheel, clipboard: clipboard,
		keys: keys, search: headless.NewSearch(), current: -1,
	}
	r.resetContent(false)
	return r
}

func (r *readerPane) Open(target readerTarget) {
	r.CloseDocument()
	r.query, r.problem, r.matches, r.current = "", "", nil, -1
	r.search.Submit(&r.content, "", false)
	if target.source != nil {
		r.observingSource = true
		initial := true
		r.releaseSource = target.source.Observe(func(document readerDocument) {
			follow := r.scroll.AtBottom()
			r.replace(document, !initial, follow)
			initial = false
		})
		return
	}
	r.replace(target.document, false, false)
}

func (r *readerPane) replace(document readerDocument, preserveScroll, follow bool) {
	r.title = strings.TrimSpace(document.Title)
	if r.title == "" {
		r.title = "Transcript entry"
	}
	r.detail = strings.TrimSpace(document.Detail)
	r.resetContent(preserveScroll)
	presentation := BlockPresentation{Theme: r.theme, Glyphs: r.glyphs, Syntax: r.syntax}
	for _, section := range document.Sections {
		blocks := renderToolSections(presentation, []ToolSection{section}, false)
		for _, block := range blocks {
			id := r.content.Append(newReaderSectionBlock(r.theme, section.Title, block))
			r.content.Finish(id)
		}
	}
	if r.content.Len() == 0 {
		id := r.content.Append(&kit.Message{Theme: r.theme, Body: "No content is available for this entry."})
		r.content.Finish(id)
	}
	if follow {
		r.scroll.ToBottom()
	}
	r.selection.Clear()
	if r.query != "" {
		r.search.Submit(&r.content, r.query, false)
	}
}

func (r *readerPane) resetContent(preserveScroll bool) {
	r.interruptSelectionGesture()
	r.content = headless.Transcript{}
	if !preserveScroll {
		r.scroll = headless.Scroll{}
		r.scroll.Wheel(r.wheel)
		r.scroll.ToTop()
	}
	r.selection = headless.Selection{}
	r.view = kit.Transcript{
		Content: &r.content, Scroll: &r.scroll, Selection: &r.selection,
		Theme: r.theme, Glyphs: r.glyphs, Keys: r.keys, Current: -1,
	}
}

func (r *readerPane) Draw(frame headless.Frame) {
	width, height := frame.Size()
	if width <= 0 || height <= 0 {
		return
	}
	rows := frame.Subs((layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
		{Size: layout.Fixed(1)},
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(1)},
	}))
	kit.Label{Text: r.title, Style: r.theme.Strong, Ellipsis: r.glyphs.Ellipsis}.Draw(rows[0].View)
	if r.detail != "" {
		kit.Label{Text: r.detail, Style: r.theme.Subtle, Align: layout.End, Ellipsis: r.glyphs.Ellipsis}.Draw(rows[0].View)
	}
	r.view.Matches, r.view.Current = r.matches, r.current
	r.view.Draw(rows[1])
	kit.Label{Text: r.footer(max(height-2, 1)), Style: r.theme.Subtle, Align: layout.End, Ellipsis: r.glyphs.Ellipsis}.Draw(rows[2].View)
}

func (r *readerPane) footer(window int) string {
	if r.problem != "" {
		return r.problem
	}
	total := r.content.Height()
	if total <= 0 {
		return "esc close"
	}
	first := min(r.scroll.Offset()+1, total)
	last := min(r.scroll.Offset()+window, total)
	percent := min((last*100)/total, 100)
	search := ""
	if r.query != "" {
		search = fmt.Sprintf(" · %d matches", len(r.matches))
	}
	return fmt.Sprintf("rows %d-%d/%d · %d%%%s · ctrl+f search · alt+c copy · esc close", first, last, total, percent, search)
}

func (r *readerPane) Handle(event input.Event) bool {
	if key, ok := event.(input.Key); ok && key.Down() {
		r.interruptSelectionGesture()
		action, _ := r.keys.Action(key.Chord())
		switch action {
		case readerFind:
			if r.openSearch != nil {
				r.openSearch()
			}
			return true
		case readerNext:
			r.StepMatch(1)
			return true
		case readerPrevious:
			r.StepMatch(-1)
			return true
		case readerCopy:
			r.copy()
			return true
		case readerClose:
			if r.dismiss != nil {
				r.dismiss()
			}
			return true
		}
	}
	switch event.(type) {
	case input.Paste, input.FocusOut:
		r.interruptSelectionGesture()
	}
	handled := r.view.Handle(event)
	if mouse, ok := event.(input.Mouse); ok {
		switch mouse.Action {
		case input.MouseDown:
			if mouse.Button == input.ButtonLeft && handled {
				r.selectionGesture.begin()
			} else {
				r.interruptSelectionGesture()
			}
		case input.MouseDrag:
			if mouse.Button != input.ButtonLeft {
				r.interruptSelectionGesture()
			}
		case input.MouseUp:
			owned := mouse.Button == input.ButtonLeft && r.selectionGesture.release()
			if mouse.Button != input.ButtonLeft {
				r.interruptSelectionGesture()
			}
			if owned && r.selection.Active() {
				r.copy()
			}
		case input.WheelUp, input.WheelDown:
			r.interruptSelectionGesture()
		}
	}
	return handled
}

func (r *readerPane) interruptSelectionGesture() {
	r.selectionGesture.cancel()
	r.selection.Done()
}

func (r *readerPane) Closed() { r.CloseDocument() }

func (r *readerPane) Find(query string) {
	r.query = strings.TrimSpace(query)
	r.problem, r.matches, r.current = "", nil, -1
	r.search.Submit(&r.content, r.query, false)
}

func (r *readerPane) AcceptSearch(result headless.Result) bool {
	if result.Query != r.query {
		return false
	}
	if result.Err != nil {
		r.problem = "search: " + result.Err.Error()
		r.matches, r.current = nil, -1
		return true
	}
	r.problem = ""
	r.matches = result.Matches
	r.current = -1
	if len(r.matches) > 0 {
		r.current = 0
	}
	return true
}

func (r *readerPane) StepMatch(delta int) bool {
	if len(r.matches) == 0 {
		return false
	}
	r.current = (r.current + delta) % len(r.matches)
	if r.current < 0 {
		r.current += len(r.matches)
	}
	return true
}

func (r *readerPane) SearchResults() <-chan headless.Result { return r.search.Results() }

func (r *readerPane) ObservingSource() bool {
	return r != nil && r.observingSource
}

func (r *readerPane) copy() {
	value := r.selection.Text(&r.content)
	if value == "" {
		value = copyableRowsText(r.content.Rows(r.content.StartRow(), r.content.Height()))
	}
	if value == "" || r.clipboard == nil || !r.clipboard.Copy(value) {
		return
	}
	if r.onCopied != nil {
		r.onCopied()
	}
}

func (r *readerPane) CloseDocument() {
	if r == nil {
		return
	}
	r.interruptSelectionGesture()
	release := r.releaseSource
	r.releaseSource = nil
	r.observingSource = false
	if release != nil {
		release()
	}
}

func (r *readerPane) Shutdown() {
	r.CloseDocument()
	r.search.Close()
}

type readerSectionBlock struct {
	theme   kit.Theme
	title   string
	content headless.Block
}

var _ headless.Copyable = (*readerSectionBlock)(nil)

func newReaderSectionBlock(theme kit.Theme, title string, content headless.Block) *readerSectionBlock {
	return &readerSectionBlock{theme: theme, title: strings.TrimSpace(title), content: content}
}

func (r *readerSectionBlock) Measure(width int) int {
	if r == nil || r.content == nil {
		return 0
	}
	return layout.Sum(r.headingRows(), r.content.Measure(width), 1)
}

func (r *readerSectionBlock) Draw(view grid.View) {
	if r == nil || r.content == nil {
		return
	}
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	y := r.headingRows()
	if y > 0 {
		view.Text(0, 0, r.title, r.theme.Strong)
	}
	r.content.Draw(view.Sub(grid.Rect(0, y, width, max(height-y-1, 0))))
}

func (r *readerSectionBlock) Rows(width int) []text.Row {
	if r == nil || r.content == nil {
		return nil
	}
	rows := make([]text.Row, 0, r.Measure(width))
	if r.title != "" {
		rows = append(rows, text.Row{Text: r.title})
	}
	if copyable, ok := r.content.(headless.Copyable); ok {
		rows = append(rows, copyable.Rows(width)...)
	} else {
		rows = append(rows, make([]text.Row, r.content.Measure(width))...)
	}
	return append(rows, text.Row{})
}

func (r *readerSectionBlock) headingRows() int {
	if r.title == "" {
		return 0
	}
	return 1
}
