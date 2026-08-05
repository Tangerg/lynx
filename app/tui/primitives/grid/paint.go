package grid

import (
	"image"
	"strconv"
)

const (
	sgrReset  = "\x1b[0m"
	osc8Open  = "\x1b]8;;"
	osc8Close = "\x1b]8;;\x1b\\"
	stringEnd = "\x1b\\"

	// Synchronized output. A frame wrapped in these is applied by the terminal in
	// one go, which is what keeps a multiplexer from showing a half-drawn frame.
	beginSync = "\x1b[?2026h"
	endSync   = "\x1b[?2026l"
)

// painter accumulates one frame's escape stream while tracking the terminal
// state it has already established — style, hyperlink and cursor column — so
// nothing is re-stated that is already true.
//
// It exists because every way of emitting cells wants exactly this bookkeeping:
// a full repaint, a diff, and the rows a scroll exposed differ only in which
// cells they consider dirty, not in how a cell reaches the wire.
type painter struct {
	out   []byte
	style Style
	link  string
	// at is where the terminal's cursor sits, and known says whether that is
	// worth believing. A run of adjacent cells needs no positioning.
	at    image.Point
	known bool
	// begun records that the frame has stated its style baseline, which also
	// means the frame is going to write something.
	begun bool
}

// restart empties the buffer and forgets everything established for the previous
// frame.
func (p *painter) restart() {
	p.out = p.out[:0]
	p.style = Style{}
	p.link = ""
	p.known = false
	p.begun = false
}

// begin states the default style, once per frame, before the first cell.
//
// The terminal's style at frame start is not knowable — another program may have
// written to it — so a frame that writes anything begins by making it knowable.
func (p *painter) begin() {
	if p.begun {
		return
	}
	p.begun = true
	p.out = append(p.out, sgrReset...)
	p.style = Style{}
	p.known = false
}

// end leaves the terminal with no hyperlink open and the default style, so
// whatever writes next — this program or the shell after it — starts clean.
func (p *painter) end() {
	if !p.begun {
		return
	}
	p.closeLink()
	p.out = append(p.out, sgrReset...)
}

// adopt continues from the terminal state another painter established.
func (p *painter) adopt(other *painter) {
	p.style = other.style
	p.link = other.link
	p.at = other.at
	p.known = other.known
	p.begun = p.begun || other.begun
}

// forcePos makes the next moveTo emit even if it targets where the cursor is
// believed to be. Sequences whose landing position is terminal-specific — a
// scrolling-region change, for one — leave that belief unfounded.
func (p *painter) forcePos() { p.known = false }

// moveTo positions the cursor unless it is already there.
func (p *painter) moveTo(x, y int) {
	if p.known && p.at.X == x && p.at.Y == y {
		return
	}
	p.out = append(p.out, '\x1b', '[')
	p.out = strconv.AppendInt(p.out, int64(y)+1, 10)
	p.out = append(p.out, ';')
	p.out = strconv.AppendInt(p.out, int64(x)+1, 10)
	p.out = append(p.out, 'H')
	p.at = image.Pt(x, y)
	p.known = true
}

// cell writes one cell unit spanning width columns, emitting only the state
// changes it needs first. It assumes the cursor is already in place.
func (p *painter) cell(c *Cell, width int) {
	if c.Style != p.style {
		p.appendStyle(c.Style)
	}
	p.appendLink(c.Link)
	if c.Content == "" {
		for range width {
			p.out = append(p.out, ' ')
		}
	} else {
		p.out = append(p.out, c.Content...)
	}
	p.at.X += width
}

// appendStyle emits the SGR that establishes next.
//
// Reset-then-set rather than a minimal difference: one deterministic sequence per
// change is shorter to reason about than a per-attribute diff, and terminals
// disagree about how to turn individual attributes off.
func (p *painter) appendStyle(next Style) {
	p.style = next
	if next == (Style{}) {
		p.out = append(p.out, sgrReset...)
		return
	}
	p.out = append(p.out, "\x1b[0"...)
	for _, a := range attrCodes {
		if next.Attr.Has(a.attr) {
			p.out = append(p.out, ';')
			p.out = strconv.AppendInt(p.out, a.code, 10)
		}
	}
	if !next.FG.Default() {
		p.appendColor(38, next.FG.rgb)
	}
	if !next.BG.Default() {
		p.appendColor(48, next.BG.rgb)
	}
	p.out = append(p.out, 'm')
}

func (p *painter) appendColor(base int64, c RGB) {
	p.out = append(p.out, ';')
	p.out = strconv.AppendInt(p.out, base, 10)
	p.out = append(p.out, ";2;"...)
	p.out = strconv.AppendInt(p.out, int64(c.R), 10)
	p.out = append(p.out, ';')
	p.out = strconv.AppendInt(p.out, int64(c.G), 10)
	p.out = append(p.out, ';')
	p.out = strconv.AppendInt(p.out, int64(c.B), 10)
}

// appendLink closes the open hyperlink and opens target.
//
// A target carrying control bytes is dropped rather than escaped: the OSC 8
// payload is terminated by a control byte, so a target containing one could close
// the sequence early and have the rest read as terminal commands. Cells can be
// filled from tool output, which makes this a trust boundary.
func (p *painter) appendLink(target string) {
	if target == p.link {
		return
	}
	if p.link != "" {
		p.out = append(p.out, osc8Close...)
	}
	p.link = ""
	if target == "" || !printableTarget(target) {
		return
	}
	p.out = append(p.out, osc8Open...)
	p.out = append(p.out, target...)
	p.out = append(p.out, stringEnd...)
	p.link = target
}

// closeLink leaves no hyperlink open at the end of a frame or a row.
func (p *painter) closeLink() { p.appendLink("") }

// paint emits every cell of rows [y0, y1) that dirty accepts, positioning the
// cursor only where a run breaks.
//
// dirty is given the index of the unit's head cell and its width so it can judge
// a wide pair as one thing: if either half changed, the head glyph has to be
// rewritten, and repainting only the trailing half would print nothing.
func (p *painter) paint(s *Surface, y0, y1 int, dirty func(i, width int) bool) {
	for y := max(y0, 0); y < min(y1, s.h); y++ {
		for x := 0; x < s.w; {
			i := y*s.w + x
			c := &s.cells[i]
			width := 1
			if c.span == spanWide && x+1 < s.w {
				width = 2
			}
			if !dirty(i, width) {
				x += width
				continue
			}
			p.begin()
			p.moveTo(x, y)
			p.cell(c, width)
			x += width
		}
	}
}

// changedAgainst reports cells that differ from the same position in prev.
func changedAgainst(prev, next *Surface) func(i, width int) bool {
	return func(i, width int) bool {
		if next.cells[i] != prev.cells[i] {
			return true
		}
		return width == 2 && next.cells[i+1] != prev.cells[i+1]
	}
}

// changedAgainstBlank reports cells that are not already blank, for rows the
// terminal has just cleared for us.
func changedAgainstBlank(next *Surface) func(i, width int) bool {
	return func(i, width int) bool {
		if next.cells[i] != (Cell{}) {
			return true
		}
		return width == 2 && next.cells[i+1] != (Cell{span: spanTrail})
	}
}

// everything accepts every cell, for a full repaint.
func everything(int, int) bool { return true }

var attrCodes = [...]struct {
	attr Attr
	code int64
}{
	{Bold, 1},
	{Dim, 2},
	{Italic, 3},
	{Underline, 4},
	{Reverse, 7},
	{Strike, 9},
}

func printableTarget(target string) bool {
	for i := range len(target) {
		if target[i] < 0x20 || target[i] == 0x7f {
			return false
		}
	}
	return true
}

// EncodeRow renders one row of cells as inline terminal text: style and
// hyperlink transitions and printable graphemes, and nothing that moves the
// cursor or erases anything.
//
// It is how a finished transcript line is printed into the terminal's own
// scrollback, where the line must survive on its own with no screen to address.
// The result always closes an open hyperlink and returns to the default style, so
// rows can be concatenated safely.
func EncodeRow(cells []Cell) string {
	cells = trimBlankTail(cells)
	var p painter
	for i := 0; i < len(cells); {
		c := &cells[i]
		if c.span == spanTrail {
			i++
			continue
		}
		width := 1
		if c.span == spanWide && i+1 < len(cells) {
			width = 2
		}
		p.cell(c, width)
		i += width
	}
	p.closeLink()
	if p.style != (Style{}) {
		p.out = append(p.out, sgrReset...)
	}
	return string(p.out)
}

// trimBlankTail drops the run of wholly default blank cells at the end of a row.
//
// Printing them costs bytes for nothing, and on a row as wide as the terminal it
// pushes the cursor onto the next line before the caller asked for one. Only the
// zero cell is dropped: a blank carrying a background colour or a hyperlink is
// visible, and the trailing half of a wide cluster must stay with its head.
func trimBlankTail(cells []Cell) []Cell {
	end := len(cells)
	for end > 0 && cells[end-1] == (Cell{}) {
		end--
	}
	return cells[:end]
}
