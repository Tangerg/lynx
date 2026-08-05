// Package text lays styled text out in terminal columns: measuring it, wrapping
// it, truncating it, and drawing it onto a [grid.View].
//
// Everything here counts columns rather than bytes or runes. A CJK or emoji
// cluster is two columns wide and is never split; a combining mark is none. Text
// that is measured one way and drawn another is the source of every misaligned
// terminal UI, so measuring and drawing live in the same place and agree by
// construction.
package text

import (
	"iter"
	"strings"

	"github.com/rivo/uniseg"

	"github.com/Tangerg/lynx/app/tui/primitives/grid"
)

// TabStop is how far apart tab stops are.
//
// Eight, because that is where a terminal would have put them: the output being
// rendered was usually formatted by a program writing to a terminal, and lining
// its columns up means agreeing with the assumption it made.
const TabStop = 8

// Span is a run of text sharing one style.
type Span struct {
	Text  string
	Style grid.Style
}

// Line is one logical line of styled text — logical in that it has no width yet.
// Wrapping turns it into however many rows it needs.
type Line []Span

// Of is the one-span line for a piece of plain styled text.
func Of(s string, style grid.Style) Line {
	if s == "" {
		return nil
	}
	return Line{{Text: s, Style: style}}
}

// String is the line's text with the styling dropped.
func (l Line) String() string {
	if len(l) == 1 {
		return l[0].Text
	}
	var b strings.Builder
	for _, s := range l {
		b.WriteString(s.Text)
	}
	return b.String()
}

// Width is how many columns the line would occupy unwrapped, with tabs expanded.
func (l Line) Width() int {
	col := 0
	for _, s := range l {
		col = advance(s.Text, col)
	}
	return col
}

// Draw writes the line onto v at (x, y) and returns how many columns it advanced.
// Tabs are expanded from the line's own start, not from the view's, so a line
// drawn at an indent keeps the column relationships it was written with.
func (l Line) Draw(v grid.View, x, y int) int {
	col := 0
	for _, s := range l {
		for _, piece := range expand(s.Text, &col) {
			v.Text(x+piece.at, y, piece.text, s.Style)
		}
	}
	return col
}

// Wrapped is one physical row produced by wrapping a [Line].
type Wrapped struct {
	Line Line
	// Joined marks a row that continues the line above it rather than starting a
	// line of its own. Anything rejoining rows — copying a selection, say — needs
	// to know which line breaks were the text's and which were the width's.
	Joined bool
}

// Width is how many columns the row occupies.
func (w Wrapped) Width() int { return w.Line.Width() }

// Draw writes the row onto v at (x, y).
func (w Wrapped) Draw(v grid.View, x, y int) int { return w.Line.Draw(v, x, y) }

// Wrap breaks the line into rows of at most width columns.
//
// Breaks are preferred at spaces, and a word longer than the width is broken
// between grapheme clusters instead. The run of spaces at a break is consumed:
// it hangs off neither the end of one row nor the start of the next. Styles
// survive every break.
//
// A width of zero or less returns the line whole: a caller with no width to lay
// out in is better served by text it can measure than by text silently thrown
// away.
func (l Line) Wrap(width int) []Wrapped {
	if width <= 0 {
		return []Wrapped{{Line: l}}
	}
	units := flatten(l)
	if len(units) == 0 {
		return []Wrapped{{Line: l}}
	}
	w := wrapper{width: width}
	for i, n := 0, len(units); i < n; {
		if units[i].space {
			w.hold(units[i])
			i++
			continue
		}
		// A word is a maximal run of clusters with no break opportunity in it.
		end, wordWidth := i, 0
		for end < n && !units[end].space {
			wordWidth += units[end].width
			end++
		}
		i = w.word(units, i, end, wordWidth)
	}
	return w.finish()
}

// wrapper accumulates rows. Held spaces are kept out of the current row until
// something follows them, which is what lets a break consume them.
type wrapper struct {
	width int
	rows  []Wrapped

	row      []unit
	rowWidth int
	held     []unit
	heldW    int
}

func (w *wrapper) hold(u unit) {
	w.held = append(w.held, u)
	w.heldW += u.width
}

func (w *wrapper) place(u unit) {
	w.row = append(w.row, u)
	w.rowWidth += u.width
}

func (w *wrapper) takeHeld() {
	w.row = append(w.row, w.held...)
	w.rowWidth += w.heldW
	w.dropHeld()
}

func (w *wrapper) dropHeld() { w.held, w.heldW = nil, 0 }

func (w *wrapper) breakRow() {
	w.rows = append(w.rows, Wrapped{Line: join(w.row), Joined: len(w.rows) > 0})
	w.row, w.rowWidth = nil, 0
}

// word places units[from:to] and returns the index to continue from.
func (w *wrapper) word(units []unit, from, to, wordWidth int) int {
	switch {
	case w.rowWidth+w.heldW+wordWidth <= w.width:
		// Fits after the spaces that preceded it.
		w.takeHeld()
		for ; from < to; from++ {
			w.place(units[from])
		}
	case wordWidth <= w.width:
		// Fits on a row of its own: break before it and drop the spaces.
		w.dropHeld()
		if len(w.row) > 0 {
			w.breakRow()
		}
		for ; from < to; from++ {
			w.place(units[from])
		}
	default:
		from = w.hardBreak(units, from, to)
	}
	return from
}

// hardBreak splits a word that is wider than a whole row.
func (w *wrapper) hardBreak(units []unit, from, to int) int {
	if len(w.held) > 0 {
		if w.rowWidth+w.heldW+units[from].width <= w.width {
			w.takeHeld()
		} else {
			w.dropHeld()
			if len(w.row) > 0 {
				w.breakRow()
			}
		}
	}
	for from < to {
		u := units[from]
		switch {
		case w.rowWidth+u.width <= w.width:
			w.place(u)
			from++
		case len(w.row) == 0:
			// A cluster wider than the whole row — a double-width one where the
			// width is one. It gets a row to itself and overflows it, because the
			// alternative is dropping it forever.
			w.place(u)
			from++
			w.breakRow()
		default:
			// Leaves the row a column short rather than splitting a wide cluster.
			w.breakRow()
		}
	}
	return from
}

// finish takes whatever trailing spaces fit and closes the last row.
func (w *wrapper) finish() []Wrapped {
	for _, u := range w.held {
		if w.rowWidth+u.width > w.width {
			break
		}
		w.place(u)
	}
	if len(w.row) > 0 {
		w.breakRow()
	}
	if len(w.rows) == 0 {
		w.rows = append(w.rows, Wrapped{})
	}
	return w.rows
}

// WrapAll wraps lines in order. Every line starts a row of its own, so a blank
// line stays a blank row.
func WrapAll(lines []Line, width int) []Wrapped {
	var out []Wrapped
	for _, l := range lines {
		out = append(out, l.Wrap(width)...)
	}
	return out
}

// Truncate cuts the line to at most width columns, ending it with ellipsis when
// anything was cut. The ellipsis takes the style of the last text that survived,
// so it reads as part of the sentence it is ending.
//
// The result can fall a column short of width: a cut never splits a wide cluster.
func (l Line) Truncate(width int, ellipsis string) Line {
	if width <= 0 {
		return nil
	}
	if l.Width() <= width {
		return l
	}
	if Width(ellipsis) > width {
		ellipsis = prefix(ellipsis, width)
	}
	budget := width - Width(ellipsis)

	units := flatten(l)
	kept := make([]unit, 0, len(units))
	used := 0
	style := grid.Style{}
	for _, u := range units {
		if used+u.width > budget {
			break
		}
		kept = append(kept, u)
		used += u.width
		style = u.style
	}
	out := join(kept)
	if ellipsis == "" {
		return out
	}
	if n := len(out); n > 0 && out[n-1].Style == style {
		out[n-1].Text += ellipsis
		return out
	}
	return append(out, Span{Text: ellipsis, Style: style})
}

// Width is how many columns s would occupy, with tabs expanded from column zero.
func Width(s string) int { return advance(s, 0) }

// Truncate cuts plain text to at most width columns, ending it with ellipsis
// when anything was cut.
func Truncate(s string, width int, ellipsis string) string {
	if width <= 0 {
		return ""
	}
	if Width(s) <= width {
		return s
	}
	if Width(ellipsis) > width {
		return prefix(ellipsis, width)
	}
	return prefix(s, width-Width(ellipsis)) + ellipsis
}

// unit is one grapheme cluster with everything wrapping needs to know about it.
type unit struct {
	cluster string
	style   grid.Style
	width   int
	// space marks a break opportunity. A tab is one, and is also the reason a
	// unit's width is not derivable from its cluster alone.
	space bool
}

// flatten turns a line into units, expanding tabs against the running column.
func flatten(l Line) []unit {
	var units []unit
	col := 0
	for _, s := range l {
		g := uniseg.NewGraphemes(s.Text)
		for g.Next() {
			cluster := g.Str()
			switch {
			case cluster == "\t":
				n := TabStop - col%TabStop
				for range n {
					units = append(units, unit{cluster: " ", style: s.Style, width: 1, space: true})
				}
				col += n
			case dropped(cluster):
				// A control character has no width to lay out and no business
				// reaching a cell.
			case cluster == " ":
				units = append(units, unit{cluster: " ", style: s.Style, width: 1, space: true})
				col++
			default:
				w := clusterWidth(cluster)
				units = append(units, unit{cluster: cluster, style: s.Style, width: w})
				col += w
			}
		}
	}
	return units
}

// join rebuilds a line from units, merging neighbours that share a style.
func join(units []unit) Line {
	var out Line
	for _, u := range units {
		if n := len(out); n > 0 && out[n-1].Style == u.style {
			out[n-1].Text += u.cluster
			continue
		}
		out = append(out, Span{Text: u.cluster, Style: u.style})
	}
	return out
}

// piece is a run of text and the column it starts at, after tab expansion.
type piece struct {
	at   int
	text string
}

// expand splits s into drawable pieces, turning tabs into the gaps they stand
// for and advancing col past everything it produced.
func expand(s string, col *int) []piece {
	if !strings.ContainsAny(s, "\t") {
		p := []piece{{at: *col, text: s}}
		*col = advance(s, *col)
		return p
	}
	var pieces []piece
	var run strings.Builder
	start := *col
	flush := func() {
		if run.Len() == 0 {
			return
		}
		pieces = append(pieces, piece{at: start, text: run.String()})
		run.Reset()
	}
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		cluster := g.Str()
		if cluster == "\t" {
			flush()
			*col += TabStop - *col%TabStop
			start = *col
			continue
		}
		if run.Len() == 0 {
			start = *col
		}
		run.WriteString(cluster)
		*col += clusterWidth(cluster)
	}
	flush()
	return pieces
}

// advance is where col ends up after s, with tabs expanded.
func advance(s string, col int) int {
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		if cluster := g.Str(); cluster == "\t" {
			col += TabStop - col%TabStop
		} else {
			col += clusterWidth(cluster)
		}
	}
	return col
}

// dropped reports whether a cluster is discarded rather than laid out. Measuring
// has to agree with drawing about this, or a line's reported width will not be the
// width it takes.
func dropped(cluster string) bool { return cluster != "\t" && isControl(cluster) }

// prefix is the longest prefix of s, cut between clusters, that fits in budget.
func prefix(s string, budget int) string {
	col, end := 0, 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		w := clusterWidth(g.Str())
		if col+w > budget {
			break
		}
		col += w
		_, end = g.Positions()
	}
	return s[:end]
}

// clusterWidth is a cluster's column count. It defers to the grid, so text is
// measured here exactly as it will be drawn there.
func clusterWidth(cluster string) int { return grid.ClusterWidth(cluster) }

// isControl reports whether a cluster is a control character. A tab is one, and
// is handled before this is asked; the rest have no width and no business
// reaching a cell.
func isControl(cluster string) bool {
	return cluster != "" && (cluster[0] < 0x20 || cluster[0] == 0x7f)
}

// Clusters iterates the grapheme clusters of s with the byte offset each starts at.
//
// It is what anything holding a cursor into text needs. A cursor cannot live on a
// rune boundary: a letter and the accent that modifies it are two runes and one
// thing on screen, and a cursor between them has no position a terminal could show.
func Clusters(s string) iter.Seq2[int, string] {
	return func(yield func(int, string) bool) {
		at, state := 0, -1
		var cluster string
		for len(s) > 0 {
			cluster, s, _, state = uniseg.StepString(s, state)
			if !yield(at, cluster) {
				return
			}
			at += len(cluster)
		}
	}
}

// NextCluster is the byte offset after the cluster at i, or len(s) at the end.
func NextCluster(s string, i int) int {
	if i < 0 {
		return 0
	}
	if i >= len(s) {
		return len(s)
	}
	for at, cluster := range Clusters(s[i:]) {
		_ = at
		return i + len(cluster)
	}
	return len(s)
}

// PrevCluster is the byte offset of the cluster ending at i, or zero at the start.
func PrevCluster(s string, i int) int {
	if i <= 0 {
		return 0
	}
	i = min(i, len(s))
	last := 0
	for at := range Clusters(s[:i]) {
		last = at
	}
	return last
}

// ColumnOf is how many columns of s sit before the byte offset i.
func ColumnOf(s string, i int) int {
	col := 0
	for at, cluster := range Clusters(s) {
		if at >= i {
			break
		}
		if cluster == "\t" {
			col += TabStop - col%TabStop
			continue
		}
		col += clusterWidth(cluster)
	}
	return col
}

// OffsetAt is the byte offset of the cluster boundary nearest to column col,
// without going past it. It is how a click, or a cursor moving between lines of
// different lengths, finds where it lands.
func OffsetAt(s string, col int) int {
	if col <= 0 {
		return 0
	}
	at, width := 0, 0
	for offset, cluster := range Clusters(s) {
		step := clusterWidth(cluster)
		if cluster == "\t" {
			step = TabStop - width%TabStop
		}
		if width+step > col {
			return offset
		}
		width += step
		at = offset + len(cluster)
	}
	return at
}
