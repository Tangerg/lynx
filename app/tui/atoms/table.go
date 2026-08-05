package atoms

import (
	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/text"
)

// Column is one column of a [Table].
type Column struct {
	Title string
	Align Align
	// Width is an exact number of columns. Zero lets the column take a share of
	// what is left, weighted by Flex.
	Width int
	// Flex is this column's share of the space the fixed columns did not take. A
	// column with neither a width nor a flex share gets one share, because a column
	// nobody sized still has to be visible.
	Flex int
	// Min is a floor on a flexible column, so it does not collapse to nothing on a
	// narrow terminal.
	Min int
}

// Table lays out rows of cells in columns.
//
// It is layout, not storage: the caller says how many rows there are and what a cell
// contains, and the table works out where each one goes. That keeps the row data
// wherever it already lives instead of copied into a widget, and it means a cell can
// be as elaborate as its own draw function likes.
type Table struct {
	Columns []Column
	// Rows is how many rows there are.
	Rows int
	// Cell is the text of one cell, and how it is styled.
	Cell func(row, column int) (string, grid.Style)
	// DrawCell, when set, takes over a cell entirely: it is handed a view of exactly
	// the cell's box. It wins over Cell, for the cells that need more than text.
	DrawCell func(v grid.View, row, column int)
	// Gap is the space between columns. Zero uses one column, which is the least
	// that still reads as two columns rather than one.
	Gap int
	// Header draws the column titles in the first row.
	Header      bool
	HeaderStyle grid.Style
	// RowStyle styles a whole row, for banding or for a selection.
	//
	// It is merged into each cell's own style rather than painted underneath it: a
	// cell drawn over a filled row replaces what was there, background and all, so a
	// band painted first would survive only in the gaps between cells. A cell drawn
	// by DrawCell is on its own — it owns its box, background included.
	RowStyle func(row int) grid.Style
}

// Widths works out each column's width for a total, which a caller needs when it is
// aligning something else to the same grid.
func (t Table) Widths(total int) []int {
	gap := max(t.Gap, 1)
	widths := make([]int, len(t.Columns))
	left := total - gap*max(len(t.Columns)-1, 0)
	shares := 0

	for i, c := range t.Columns {
		if c.Width > 0 {
			widths[i] = min(c.Width, max(left, 0))
			left -= widths[i]
			continue
		}
		shares += max(c.Flex, 1)
	}
	if shares == 0 {
		return widths
	}
	remainder := max(left, 0)
	last := -1
	used := 0
	for i, c := range t.Columns {
		if c.Width > 0 {
			continue
		}
		widths[i] = max(remainder*max(c.Flex, 1)/shares, c.Min)
		used += widths[i]
		last = i
	}
	// The rounding remainder goes to the last flexible column rather than being
	// lost, so the table fills its width exactly and its right edge lines up with
	// whatever is drawn beside it.
	if last >= 0 && used < remainder {
		widths[last] += remainder - used
	}
	return widths
}

// Height is the rows plus the header, which is what a container measures against.
func (t Table) Height(int) int {
	if t.Header {
		return t.Rows + 1
	}
	return t.Rows
}

// Draw paints the header and as many rows as fit.
func (t Table) Draw(v grid.View) {
	width, height := v.Size()
	if width <= 0 || height <= 0 || len(t.Columns) == 0 {
		return
	}
	widths := t.Widths(width)
	gap := max(t.Gap, 1)

	y := 0
	if t.Header {
		t.drawRow(v, y, widths, gap, func(col int, cell grid.View) {
			c := t.Columns[col]
			Label{Text: c.Title, Style: t.HeaderStyle, Align: c.Align, Ellipsis: "…"}.Draw(cell)
		})
		y++
	}
	for row := 0; row < t.Rows && y < height; row, y = row+1, y+1 {
		var band grid.Style
		if t.RowStyle != nil {
			band = t.RowStyle(row)
		}
		if band != (grid.Style{}) {
			v.Fill(grid.Rect(0, y, width, 1), band)
		}
		t.drawRow(v, y, widths, gap, func(col int, cell grid.View) {
			t.drawCell(cell, row, col, band)
		})
	}
}

// drawRow hands each column's box to draw.
func (t Table) drawRow(v grid.View, y int, widths []int, gap int, draw func(col int, cell grid.View)) {
	x := 0
	for col, w := range widths {
		if w > 0 {
			draw(col, v.Sub(grid.Rect(x, y, w, 1)))
		}
		x += w + gap
	}
}

func (t Table) drawCell(cell grid.View, row, col int, band grid.Style) {
	if t.DrawCell != nil {
		t.DrawCell(cell, row, col)
		return
	}
	if t.Cell == nil {
		return
	}
	content, style := t.Cell(row, col)
	c := t.Columns[col]
	width, _ := cell.Size()
	shown := text.Truncate(content, width, "…")
	cell.Text(c.Align.offset(width, text.Width(shown)), 0, shown, band.Merge(style))
}
