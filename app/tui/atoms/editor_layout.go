package atoms

import (
	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/text"
)

// editorRow is one visual row: which logical line it came from, and the slice of
// that line it shows.
type editorRow struct {
	line       int
	start, end int
	// joined marks a row that continues the line above rather than starting one.
	joined bool
}

// editorLayout is the wrap of an editor's text at one width.
//
// The editor wraps its own text rather than asking the text package to, because it
// needs the byte offset each row starts at: the cursor lives at an offset, and
// moving it down the screen means finding the offset that sits under it. Measuring
// with one wrap and drawing with another is how a cursor ends up a column away from
// the character it is on.
type editorLayout struct {
	rows  []editorRow
	width int
	stale bool
}

// rowsFor lays the lines out at a width, reusing the last layout when nothing that
// matters has changed.
func (l *editorLayout) rowsFor(lines []string, width int) []editorRow {
	if !l.stale && l.width == width && l.rows != nil {
		return l.rows
	}
	l.rows = l.rows[:0]
	for i, line := range lines {
		l.wrapLine(i, line, width)
	}
	l.width, l.stale = width, false
	return l.rows
}

// wrapLine appends the rows one logical line occupies.
//
// It breaks at spaces when it can and between clusters when it cannot, which is what
// the text package does, so a field and the prose beside it break in the same
// places. A line with nothing in it still gets a row: a blank line in a composer is
// a blank line on screen.
func (l *editorLayout) wrapLine(index int, line string, width int) {
	if width <= 0 || line == "" {
		l.rows = append(l.rows, editorRow{line: index, start: 0, end: len(line)})
		return
	}

	start := 0
	column := 0
	// lastBreak is where the row could end instead, and lastBreakColumn what it
	// would be worth: the offset after the most recent space.
	lastBreak := -1
	for at, cluster := range text.Clusters(line) {
		step := grid.ClusterWidth(cluster)
		if cluster == "\t" {
			step = text.TabStop - column%text.TabStop
		}
		if column+step > width && at > start {
			end := at
			if lastBreak > start {
				end = lastBreak
			}
			l.rows = append(l.rows, editorRow{
				line: index, start: start, end: end, joined: start > 0,
			})
			start = end
			column = text.ColumnOf(line[start:at], at-start)
		}
		if cluster == " " {
			lastBreak = at + len(cluster)
		}
		column += step
	}
	l.rows = append(l.rows, editorRow{line: index, start: start, end: len(line), joined: start > 0})
}

// rowOf is the index of the visual row holding a cursor, and the column within it.
func rowOf(rows []editorRow, lines []string, line, col int) (row, column int) {
	for i, r := range rows {
		if r.line != line {
			continue
		}
		// The end of a row is the start of the next, so a cursor there belongs to the
		// next row — except on the last row of a line, where there is no next and the
		// cursor sits after the final character.
		if col < r.end || (col == r.end && lastRowOfLine(rows, i)) {
			return i, text.ColumnOf(lines[line][r.start:r.end], col-r.start)
		}
	}
	if len(rows) == 0 {
		return 0, 0
	}
	return len(rows) - 1, 0
}

// lastRowOfLine reports whether rows[i] is the final row of its logical line.
func lastRowOfLine(rows []editorRow, i int) bool {
	return i == len(rows)-1 || rows[i+1].line != rows[i].line
}

// offsetIn is the cursor offset that sits at a column of a row.
func offsetIn(rows []editorRow, lines []string, row, column int) (line, col int) {
	if row < 0 || row >= len(rows) {
		return 0, 0
	}
	r := rows[row]
	segment := lines[r.line][r.start:r.end]
	return r.line, r.start + text.OffsetAt(segment, column)
}

// MoveUp moves the cursor up one visual row, keeping the column it started from.
func (e *Editor) MoveUp() { e.moveRow(-1) }

// MoveDown moves the cursor down one visual row.
func (e *Editor) MoveDown() { e.moveRow(1) }

// moveRow moves the cursor by visual rows.
//
// The column the cursor was in is remembered across the whole run of movement, so
// travelling down through a short line and out the other side comes back to where it
// went in. Recomputing it each step would drag the cursor left and leave it there.
func (e *Editor) moveRow(delta int) {
	e.ensure()
	width := e.layout.width
	if width <= 0 {
		// Nothing has been drawn yet, so there are no visual rows to move through.
		// Logical lines are the best available answer.
		e.line = min(max(e.line+delta, 0), len(e.lines)-1)
		e.col = min(e.col, len(e.lines[e.line]))
		return
	}
	rows := e.layout.rowsFor(e.lines, width)
	row, column := rowOf(rows, e.lines, e.line, e.col)
	if e.wantColumn >= 0 {
		column = e.wantColumn
	}
	target := row + delta
	if target < 0 || target >= len(rows) {
		return
	}
	e.line, e.col = offsetIn(rows, e.lines, target, column)
	e.wantColumn = column
}

// Height is how many rows the field needs at a width, within its cap.
func (e *Editor) Height(width int) int {
	e.ensure()
	rows := len(e.layout.rowsFor(e.lines, width))
	if e.MaxRows > 0 {
		return min(rows, e.MaxRows)
	}
	return rows
}

// Draw paints the field and places the cursor.
func (e *Editor) Draw(v grid.View) {
	width, height := v.Size()
	if width <= 0 || height <= 0 {
		return
	}
	e.ensure()

	if e.Empty() && e.Placeholder != "" {
		v.Text(0, 0, text.Truncate(e.Placeholder, width, "…"), e.PlaceholderStyle)
		v.PlaceCursor(0, 0)
		return
	}

	rows := e.layout.rowsFor(e.lines, width)
	cursorRow, cursorColumn := rowOf(rows, e.lines, e.line, e.col)

	// The field scrolls only when it is taller than its box, and then only as far as
	// it must to keep the cursor visible: a field that jumped to the end would lose
	// the line the user is typing on.
	e.scroll.Layout(len(rows), height)
	if cursorRow < e.scroll.Offset() {
		e.scroll.By(cursorRow - e.scroll.Offset())
	} else if last := e.scroll.Offset() + height - 1; cursorRow > last {
		e.scroll.By(cursorRow - last)
	}
	first := e.scroll.Offset()

	for y := range height {
		index := first + y
		if index >= len(rows) {
			break
		}
		r := rows[index]
		text.Of(e.lines[r.line][r.start:r.end], e.Style).Draw(v, 0, y)
	}
	if y := cursorRow - first; y >= 0 && y < height {
		v.PlaceCursor(cursorColumn, y)
	}
}

// Scroll exposes the field's position, for a scrollbar beside a tall field.
func (e *Editor) Scroll() *Scroll { return &e.scroll }
