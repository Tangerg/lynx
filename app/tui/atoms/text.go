package atoms

import (
	"strings"

	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/text"
)

// Label is one line of text that does not wrap.
//
// Text too wide for its space is truncated rather than folded, because a label is
// used where exactly one row is available — a header, a status field, a table cell —
// and a label that grew to two rows would push whatever is below it off the screen.
type Label struct {
	Text  string
	Style grid.Style
	Align Align
	// Ellipsis marks a truncation. Empty means truncate silently, which is right
	// for a value the user can see in full elsewhere and wrong for prose.
	Ellipsis string
}

// Height is one row, whatever the width.
func (l Label) Height(int) int { return 1 }

// Draw writes the label into the first row of v.
func (l Label) Draw(v grid.View) {
	w, h := v.Size()
	if w <= 0 || h <= 0 {
		return
	}
	shown := text.Truncate(l.Text, w, l.Ellipsis)
	v.Text(l.Align.offset(w, text.Width(shown)), 0, shown, l.Style)
}

// Paragraph is text that wraps to the width it is given.
//
// Its height is not known until its width is, which is the whole reason [Sized]
// exists: a container has to ask before it can decide how much room to give.
type Paragraph struct {
	// Lines are the logical lines. A line's own styling survives wrapping.
	Lines []text.Line
	// Indent is held clear on the left of every row, including continuations, so a
	// wrapped paragraph reads as one block rather than as several.
	Indent int
	// MaxRows caps the height. Zero means no cap; a cap replaces the last row it
	// keeps with one ending in an ellipsis.
	MaxRows int

	// wrapped memoises the last wrap, which is asked for twice per frame — once to
	// measure and once to draw — and is the most expensive thing this widget does.
	wrapped []text.Wrapped
	atWidth int
	fresh   bool
}

// Of is a paragraph of one plain styled string. Its newlines are line breaks.
func Of(s string, style grid.Style) *Paragraph {
	return &Paragraph{Lines: linesOf(s, style)}
}

// SetText replaces the content.
func (p *Paragraph) SetText(lines []text.Line) {
	p.Lines = lines
	p.fresh = false
}

// Height is how many rows the paragraph needs at this width.
func (p *Paragraph) Height(width int) int { return len(p.rows(width)) }

// Draw writes the paragraph, one wrapped row per row of v.
func (p *Paragraph) Draw(v grid.View) {
	w, h := v.Size()
	rows := p.rows(w)
	for y, row := range rows {
		if y >= h {
			return
		}
		row.Draw(v, p.Indent, y)
	}
}

// rows is the wrap at this width, computed once per width.
func (p *Paragraph) rows(width int) []text.Wrapped {
	room := width - p.Indent
	if room <= 0 {
		return nil
	}
	if p.fresh && p.atWidth == room {
		return p.wrapped
	}
	rows := text.WrapAll(p.Lines, room)
	if p.MaxRows > 0 && len(rows) > p.MaxRows {
		rows = rows[:p.MaxRows]
		last := len(rows) - 1
		rows[last] = text.Wrapped{
			Line:   cutOff(rows[last].Line, room),
			Joined: rows[last].Joined,
		}
	}
	p.wrapped, p.atWidth, p.fresh = rows, room, true
	return rows
}

// cutOff ends a line with an ellipsis to say that content was dropped after it.
//
// Truncating to the width would not do: the last row that survived a cap usually
// fits, and a row that fits is left alone — which would leave nothing to tell the
// reader there is more.
func cutOff(l text.Line, room int) text.Line {
	const ellipsis = "…"
	budget := room - text.Width(ellipsis)
	if budget <= 0 {
		return text.Of(ellipsis, grid.Style{})
	}
	if l.Width() > budget {
		l = l.Truncate(budget, "")
	}
	style := grid.Style{}
	if n := len(l); n > 0 {
		style = l[n-1].Style
	}
	return append(l, text.Span{Text: ellipsis, Style: style})
}

// linesOf splits a string on newlines into logical lines.
func linesOf(s string, style grid.Style) []text.Line {
	var lines []text.Line
	for line := range strings.SplitSeq(s, "\n") {
		lines = append(lines, text.Of(line, style))
	}
	return lines
}
