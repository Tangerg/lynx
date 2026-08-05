package grid

import (
	"bytes"
	"image"
	"strings"
	"testing"
)

// text reads back a row as plain characters, with a dot for a blank cell, so a
// test can state what the grid looks like instead of what it contains.
func text(s *Surface, y int) string {
	var b strings.Builder
	for x := range s.w {
		c := s.CellAt(x, y)
		switch {
		case c.span == spanTrail:
		case c.Content == "":
			b.WriteByte('.')
		default:
			b.WriteString(c.Content)
		}
	}
	return b.String()
}

// flush renders one frame and returns the bytes, without the frame markers.
func flush(t *testing.T, s *Screen, cursor Cursor, draw func(View)) string {
	t.Helper()
	v := s.Frame()
	if cursor.Visible {
		v.PlaceCursor(cursor.Pos.X, cursor.Pos.Y)
	}
	if draw != nil {
		draw(v)
	}
	var buf bytes.Buffer
	if err := s.Flush(&buf); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	out := buf.String()
	if out == "" {
		return ""
	}
	if !strings.HasPrefix(out, beginSync) || !strings.HasSuffix(out, endSync) {
		t.Fatalf("frame is not wrapped for atomic application: %q", out)
	}
	return strings.TrimSuffix(strings.TrimPrefix(out, beginSync), endSync)
}

func TestZeroCellIsABlankSingleColumn(t *testing.T) {
	var c Cell
	if c.Width() != 1 || !c.Blank() {
		t.Fatalf("zero Cell = %+v, want a blank single-width cell", c)
	}
	// A freshly sized surface must already be a grid of blanks, not a grid of
	// orphaned continuation cells.
	s := NewSurface(3, 1)
	if got := text(s, 0); got != "..." {
		t.Fatalf("new surface row = %q, want blanks", got)
	}
}

func TestTextWritesAndClips(t *testing.T) {
	s := NewSurface(6, 2)
	v := s.View()

	if got := v.Text(0, 0, "hello", Style{}); got != 5 {
		t.Fatalf("advance = %d, want 5", got)
	}
	if got := text(s, 0); got != "hello." {
		t.Fatalf("row 0 = %q", got)
	}
	// Past the right edge: written where it fits, dropped where it does not.
	v.Text(4, 1, "abcd", Style{})
	if got := text(s, 1); got != "....ab" {
		t.Fatalf("row 1 = %q, want the overflow dropped", got)
	}
	// Off the bottom: no panic, and the advance is still reported so a caller
	// laying out a line gets the same answer wherever it lands.
	if got := v.Text(0, 9, "xyz", Style{}); got != 3 {
		t.Fatalf("off-surface advance = %d, want 3", got)
	}
}

func TestWideClustersOccupyTwoColumns(t *testing.T) {
	s := NewSurface(6, 1)
	if got := s.View().Text(0, 0, "中文", Style{}); got != 4 {
		t.Fatalf("advance = %d, want 4 columns for two wide clusters", got)
	}
	if s.CellAt(0, 0).Width() != 2 || s.CellAt(1, 0).Width() != 0 {
		t.Fatal("a wide cluster did not claim a head and a trailing cell")
	}
	if got := text(s, 0); got != "中文.." {
		t.Fatalf("row = %q", got)
	}
}

func TestWideClusterIsNeverSplitAtTheRightEdge(t *testing.T) {
	// Two columns: the letter takes one, and the wide cluster cannot have the one
	// that is left. Half a glyph would be worse than a gap.
	s := NewSurface(2, 1)
	s.View().Text(0, 0, "a中", Style{})
	if got := text(s, 0); got != "a." {
		t.Fatalf("row = %q, want the wide cluster dropped and its column blanked", got)
	}
	if c := s.CellAt(1, 0); c.Width() != 1 || !c.Blank() {
		t.Fatalf("cell 1 = %+v, want a blank single cell", c)
	}
	// One more column and it does fit, exactly.
	wider := NewSurface(3, 1)
	wider.View().Text(0, 0, "a中", Style{})
	if got := text(wider, 0); got != "a中" {
		t.Fatalf("row = %q, want the wide cluster to have fitted", got)
	}
}

func TestOverwritingHalfOfAWidePairBlanksTheOther(t *testing.T) {
	for _, tc := range []struct {
		name string
		at   int
		want string
	}{
		{"head", 0, "x."},
		{"trailing", 1, ".x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSurface(2, 1)
			v := s.View()
			v.Text(0, 0, "中", Style{})
			v.Text(tc.at, 0, "x", Style{})
			if got := text(s, 0); got != tc.want {
				t.Fatalf("row = %q, want %q", got, tc.want)
			}
			for x := range 2 {
				if w := s.CellAt(x, 0).Width(); w == 0 || w == 2 {
					t.Fatalf("cell %d is still half of a pair (width %d)", x, w)
				}
			}
		})
	}
}

func TestZeroWidthClusterJoinsTheCellToItsLeft(t *testing.T) {
	s := NewSurface(3, 1)
	// A combining acute arriving on its own after the letter it modifies.
	s.View().Text(0, 0, "éx", Style{})
	if got := s.CellAt(0, 0).Content; got != "é" {
		t.Fatalf("cell 0 = %q, want the mark folded into the letter", got)
	}
	if got := s.CellAt(1, 0).Content; got != "x" {
		t.Fatalf("cell 1 = %q, want the next letter to have kept its column", got)
	}
}

func TestFillStylesAndBlanks(t *testing.T) {
	s := NewSurface(4, 2)
	v := s.View()
	v.Text(0, 0, "abcd", Style{})
	style := Style{FG: RGBColor(1, 2, 3)}
	v.Fill(Rect(1, 0, 2, 1), style)
	if got := text(s, 0); got != "a..d" {
		t.Fatalf("row = %q, want the filled span blanked", got)
	}
	if got := s.CellAt(1, 0).Style; got != style {
		t.Fatalf("style = %+v, want the fill's", got)
	}
}

func TestSubViewNarrowsAndKeepsItsOwnCoordinates(t *testing.T) {
	s := NewSurface(10, 4)
	inner := s.View().Sub(Rect(2, 1, 3, 2))

	if w, h := inner.Size(); w != 3 || h != 2 {
		t.Fatalf("size = %dx%d, want 3x2", w, h)
	}
	inner.Text(0, 0, "abcdef", Style{})
	if got := text(s, 1); got != "..abc....." {
		t.Fatalf("row 1 = %q, want the write placed and clipped by the sub-view", got)
	}
	// A nested view cannot widen what it was given.
	wider := inner.Sub(Rect(-5, 0, 20, 1))
	wider.Text(0, 0, "ZZZZZZZZZZ", Style{})
	if got := text(s, 1); got != "..ZZZ....." {
		t.Fatalf("row 1 = %q, want the nested view still clipped to its parent", got)
	}
}

func TestViewSizeIsNominalAndVisibleIsClipped(t *testing.T) {
	s := NewSurface(6, 2)
	// A widget laid out half off the right edge still lays out for its whole box.
	v := s.View().Sub(Rect(4, 0, 6, 1))
	if w, _ := v.Size(); w != 6 {
		t.Fatalf("Size width = %d, want the box it was laid out into", w)
	}
	if got := v.Visible().Dx(); got != 2 {
		t.Fatalf("Visible width = %d, want only what reaches the screen", got)
	}
}

func TestZeroViewDrawsNowhere(t *testing.T) {
	var v View
	if !v.Empty() {
		t.Fatal("the zero View claims it can draw")
	}
	// None of these may panic: a widget given no room still runs its draw code.
	v.Fill(Rect(0, 0, 5, 5), Style{})
	v.Link(0, 0, 3, "https://example.test")
	if got := v.Text(0, 0, "hello", Style{}); got != 0 {
		t.Fatalf("advance = %d, want 0", got)
	}
	if v.CellAt(0, 0) != nil {
		t.Fatal("the zero View handed out a cell")
	}
	if w, h := v.Size(); w != 0 || h != 0 {
		t.Fatalf("size = %dx%d, want zero", w, h)
	}
}

func TestStyleMerge(t *testing.T) {
	base := Style{FG: RGBColor(10, 10, 10), BG: RGBColor(20, 20, 20), Attr: Bold}
	over := Style{BG: RGBColor(30, 30, 30), Attr: Underline}
	got := base.Merge(over)

	if got.FG != base.FG {
		t.Error("a default foreground in the overlay dropped the one underneath")
	}
	if got.BG != over.BG {
		t.Error("the overlay's background did not win")
	}
	if !got.Attr.Has(Bold | Underline) {
		t.Errorf("attributes = %b, want both to survive", got.Attr)
	}
}

func TestColorBlend(t *testing.T) {
	black, white := RGBColor(0, 0, 0), RGBColor(255, 255, 255)
	if got := black.Blend(white, 0.5).RGB(); got != (RGB{128, 128, 128}) {
		t.Fatalf("halfway = %+v, want mid grey", got)
	}
	if got := black.Blend(white, 2); got != white {
		t.Fatal("opacity above one did not clamp to the overlay")
	}
	// Nothing is known about what the terminal default resolves to, so a blend
	// involving it cannot invent a mixture.
	if got := (Color{}).Blend(white, 0.5); got != white {
		t.Fatal("blending from the terminal default guessed at a value")
	}
	if got := black.Blend(Color{}, 0.5); !got.Default() {
		t.Fatal("blending toward the terminal default lost it")
	}
}

func TestFirstFrameRepaintsAndAnIdenticalFrameIsSilent(t *testing.T) {
	s := NewScreen(4, 1)
	draw := func(v View) { v.Text(0, 0, "abcd", Style{}) }

	first := flush(t, s, Cursor{}, draw)
	if !strings.Contains(first, "abcd") {
		t.Fatalf("first frame = %q, want the content painted", first)
	}
	// An unchanged frame writes nothing: not the cells, not the frame markers,
	// and above all not a cursor command, which would restart the blink.
	if second := flush(t, s, Cursor{}, draw); second != "" {
		t.Fatalf("unchanged frame wrote %q, want silence", second)
	}
}

func TestDiffWritesOnlyWhatChanged(t *testing.T) {
	s := NewScreen(10, 2)
	flush(t, s, Cursor{}, func(v View) {
		v.Text(0, 0, "unchanged", Style{})
		v.Text(0, 1, "before", Style{})
	})
	out := flush(t, s, Cursor{}, func(v View) {
		v.Text(0, 0, "unchanged", Style{})
		v.Text(0, 1, "beFore", Style{})
	})

	if !strings.Contains(out, "F") {
		t.Fatalf("frame = %q, want the changed cell", out)
	}
	if strings.Contains(out, "unchanged") {
		t.Fatalf("frame = %q, want the untouched row left alone", out)
	}
	// Just the reset pair, one position, and one glyph.
	if len(out) > 20 {
		t.Fatalf("frame = %q (%d bytes), want a minimal stream", out, len(out))
	}
}

func TestCursorCommands(t *testing.T) {
	s := NewScreen(4, 2)
	at := func(x, y int) Cursor { return Cursor{Visible: true, Pos: image.Pt(x, y)} }

	first := flush(t, s, at(1, 0), func(v View) { v.Text(0, 0, "ab", Style{}) })
	if !strings.Contains(first, showCursor) {
		t.Fatalf("first frame = %q, want the cursor shown", first)
	}

	// Same cursor, same cells: silence, so the blink timer survives.
	if out := flush(t, s, at(1, 0), func(v View) { v.Text(0, 0, "ab", Style{}) }); out != "" {
		t.Fatalf("idle frame = %q, want silence", out)
	}
	// Moved: repositioned, and not re-shown.
	out := flush(t, s, at(2, 1), func(v View) { v.Text(0, 0, "ab", Style{}) })
	if !strings.Contains(out, "\x1b[2;3H") {
		t.Fatalf("frame = %q, want the cursor moved to row 2 column 3", out)
	}
	if strings.Contains(out, showCursor) {
		t.Fatalf("frame = %q, want no redundant show", out)
	}
	// Hidden: one command, and nothing else.
	out = flush(t, s, Cursor{}, func(v View) { v.Text(0, 0, "ab", Style{}) })
	if out != hideCursor {
		t.Fatalf("frame = %q, want only the hide", out)
	}
}

func TestWritingCellsReanchorsAnUnmovedCursor(t *testing.T) {
	s := NewScreen(6, 1)
	cursor := Cursor{Visible: true, Pos: image.Pt(0, 0)}
	flush(t, s, cursor, func(v View) { v.Text(0, 0, "aa", Style{}) })

	// The glyph left the terminal's cursor after it, so the frame has to say
	// where the cursor belongs even though it did not move.
	out := flush(t, s, cursor, func(v View) { v.Text(0, 0, "ab", Style{}) })
	if strings.Count(out, "\x1b[1;1H") != 1 {
		t.Fatalf("frame = %q, want the cursor re-anchored once", out)
	}
}

func TestResizeRepaintsInFull(t *testing.T) {
	s := NewScreen(4, 1)
	flush(t, s, Cursor{}, func(v View) { v.Text(0, 0, "abcd", Style{}) })
	s.Resize(6, 1)
	out := flush(t, s, Cursor{}, func(v View) { v.Text(0, 0, "abcd", Style{}) })
	if !strings.Contains(out, "abcd") {
		t.Fatalf("frame after resize = %q, want a full repaint", out)
	}
}

func TestScrollUsesTheTerminalsOwnShift(t *testing.T) {
	const w, h = 24, 10
	s := NewScreen(w, h)
	rows := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india", "juliett"}
	flush(t, s, Cursor{}, func(v View) {
		for y, r := range rows {
			v.Text(0, y, r, Style{})
		}
	})

	// The same content one row higher, with a new row at the bottom: a pure shift.
	out := flush(t, s, Cursor{}, func(v View) {
		for y := range h - 1 {
			v.Text(0, y, rows[y+1], Style{})
		}
		v.Text(0, h-1, "kilo", Style{})
	})
	if !strings.Contains(out, "\x1b[1S") {
		t.Fatalf("frame = %q, want the terminal asked to scroll", out)
	}
	if !strings.Contains(out, "kilo") {
		t.Fatalf("frame = %q, want the exposed row painted", out)
	}
	for _, carried := range rows[1:] {
		if strings.Contains(out, carried) {
			t.Fatalf("frame = %q, want %q carried by the scroll rather than repainted", out, carried)
		}
	}
}

func TestScrollIsRefusedWhenItWouldCostMore(t *testing.T) {
	// Two rows of content and a one-row change: a shift would move as much as it
	// saves, so the plain diff has to win.
	s := NewScreen(20, 3)
	flush(t, s, Cursor{}, func(v View) {
		v.Text(0, 0, "one", Style{})
		v.Text(0, 1, "two", Style{})
		v.Text(0, 2, "three", Style{})
	})
	out := flush(t, s, Cursor{}, func(v View) {
		v.Text(0, 0, "one", Style{})
		v.Text(0, 1, "TWO", Style{})
		v.Text(0, 2, "three", Style{})
	})
	if strings.Contains(out, "S") && strings.Contains(out, "\x1b[") && strings.Contains(out, "\x1b[1S") {
		t.Fatalf("frame = %q, want the plain diff for a single-row edit", out)
	}
	if !strings.Contains(out, "TWO") {
		t.Fatalf("frame = %q, want the edit painted", out)
	}
}

func TestHyperlinksOpenAndClose(t *testing.T) {
	s := NewScreen(10, 1)
	out := flush(t, s, Cursor{}, func(v View) {
		v.Text(0, 0, "link", Style{})
		v.Link(0, 0, 4, "https://example.test/x")
	})
	if !strings.Contains(out, osc8Open+"https://example.test/x"+stringEnd) {
		t.Fatalf("frame = %q, want the hyperlink opened", out)
	}
	if !strings.Contains(out, osc8Close) {
		t.Fatalf("frame = %q, want the hyperlink closed", out)
	}
	if strings.LastIndex(out, osc8Open) > strings.LastIndex(out, osc8Close) {
		t.Fatalf("frame = %q, want no hyperlink left open at the end", out)
	}
}

func TestHyperlinkTargetWithControlBytesIsDropped(t *testing.T) {
	s := NewScreen(10, 1)
	// A target carrying the string terminator could close the sequence early and
	// have what follows read as terminal commands. Cells can be filled from tool
	// output, so this is a trust boundary rather than a formatting nicety.
	out := flush(t, s, Cursor{}, func(v View) {
		v.Text(0, 0, "x", Style{})
		v.Link(0, 0, 1, "https://ok\x1b\\\x1b]0;pwned\x07")
	})
	if strings.Contains(out, osc8Open) {
		t.Fatalf("frame = %q, want the unsafe target dropped", out)
	}
	if !strings.Contains(out, "x") {
		t.Fatalf("frame = %q, want the text still painted", out)
	}
}

func TestStyleIsStatedOncePerRun(t *testing.T) {
	s := NewScreen(8, 1)
	red := Style{FG: RGBColor(255, 0, 0)}
	out := flush(t, s, Cursor{}, func(v View) { v.Text(0, 0, "abcd", red) })
	// One SGR for the run, plus the frame's opening and closing resets.
	if got := strings.Count(out, "\x1b[0;38;2;255;0;0m"); got != 1 {
		t.Fatalf("frame = %q, want one style statement, got %d", out, got)
	}
}

func TestEncodeRowIsSelfContainedInlineText(t *testing.T) {
	s := NewSurface(6, 1)
	v := s.View()
	v.Text(0, 0, "hi", Style{FG: RGBColor(1, 2, 3)})
	v.Link(0, 0, 2, "https://example.test")

	row := EncodeRow(s.Row(0))
	if strings.Contains(row, "\x1b[1;") || strings.Contains(row, "H") {
		t.Fatalf("row = %q, want nothing that moves the cursor", row)
	}
	if strings.LastIndex(row, osc8Open) > strings.LastIndex(row, osc8Close) {
		t.Fatalf("row = %q, want no hyperlink left open", row)
	}
	if !strings.Contains(row, "hi") {
		t.Fatalf("row = %q, want the text", row)
	}
	// Trailing blanks are not printed: they cost bytes for nothing and, on a
	// full-width row, wrap the cursor before the caller asked it to.
	if strings.HasSuffix(row, " ") {
		t.Fatalf("row = %q, want the blank tail dropped", row)
	}
}

func TestEncodeRowSkipsTrailingHalvesOfWideClusters(t *testing.T) {
	s := NewSurface(4, 1)
	s.View().Text(0, 0, "中文", Style{})
	if got := EncodeRow(s.Row(0)); got != "中文" {
		t.Fatalf("row = %q, want each wide cluster emitted once", got)
	}
}

// failWriter fails after letting n bytes through.
type failWriter struct{ n int }

func (w *failWriter) Write(p []byte) (int, error) {
	if w.n <= 0 {
		return 0, errWrite
	}
	w.n -= len(p)
	return len(p), nil
}

var errWrite = &writeError{}

type writeError struct{}

func (*writeError) Error() string { return "write failed" }

func TestAFailedWriteForcesAFullRepaint(t *testing.T) {
	s := NewScreen(4, 1)
	s.Frame().Text(0, 0, "abcd", Style{})
	if err := s.Flush(&failWriter{}); err == nil {
		t.Fatal("Flush hid a write failure")
	}
	// Some prefix of the frame may have landed, so the terminal's contents are
	// unknown and diffing against them would be a guess.
	out := flush(t, s, Cursor{}, func(v View) { v.Text(0, 0, "abcd", Style{}) })
	if !strings.Contains(out, "abcd") {
		t.Fatalf("frame after a failed write = %q, want a full repaint", out)
	}
}

func TestInvalidateForcesAFullRepaint(t *testing.T) {
	s := NewScreen(4, 1)
	draw := func(v View) { v.Text(0, 0, "abcd", Style{}) }
	flush(t, s, Cursor{Visible: true}, draw)
	s.Invalidate()
	out := flush(t, s, Cursor{Visible: true}, draw)
	if !strings.Contains(out, "abcd") || !strings.Contains(out, showCursor) {
		t.Fatalf("frame after Invalidate = %q, want everything re-stated", out)
	}
}

func TestCopyRowsLiftsRowsBetweenSurfaces(t *testing.T) {
	src := NewSurface(4, 4)
	for y := range 4 {
		src.View().Text(0, y, strings.Repeat(string(rune('a'+y)), 4), Style{})
	}
	dst := NewSurface(4, 2)
	dst.CopyRows(src, 2, 0, 2)
	if got := text(dst, 0); got != "cccc" {
		t.Fatalf("row 0 = %q", got)
	}
	if got := text(dst, 1); got != "dddd" {
		t.Fatalf("row 1 = %q", got)
	}
	// Out-of-range rows are skipped rather than fatal, which is what lets a
	// caller lift the visible slice of an over-tall item into place.
	dst.CopyRows(src, 3, 1, 4)
	if got := text(dst, 1); got != "dddd" {
		t.Fatalf("row 1 = %q, want the copy to have stopped at the edge", got)
	}
}

func TestSurfaceMethodsTolerateANilReceiver(t *testing.T) {
	var s *Surface
	if s.CellAt(0, 0) != nil || s.Row(0) != nil {
		t.Fatal("a nil surface handed out cells")
	}
	if !s.View().Empty() {
		t.Fatal("a nil surface handed out a drawable view")
	}
}

func TestControlCharactersNeverReachACell(t *testing.T) {
	// Cells are filled from tool output and model output. A control byte stored in
	// one would be written to the terminal verbatim on the next repaint: a tab or
	// carriage return would move the cursor out from under the renderer, and an
	// escape would begin a sequence the terminal obeys.
	s := NewSurface(20, 1)
	s.View().Text(0, 0, "a\x1b]0;title\x07b\tc\rd", Style{})

	for x := range 20 {
		if c := s.CellAt(x, 0); strings.ContainsAny(c.Content, "\x1b\x07\t\r") {
			t.Fatalf("cell %d holds a control character: %q", x, c.Content)
		}
	}
	if got := text(s, 0); got != "a]0;titlebcd........" {
		t.Fatalf("row = %q, want only the printable characters, none of them shifted", got)
	}
}

func TestControlCharactersAreNotFoldedIntoTheCellBefore(t *testing.T) {
	// A zero-width cluster joins the cell to its left, and a control character
	// measures zero. Folding one in would smuggle it into a cell that looks
	// printable.
	s := NewSurface(4, 1)
	s.View().Text(0, 0, "a\x1b", Style{})
	if got := s.CellAt(0, 0).Content; got != "a" {
		t.Fatalf("cell 0 = %q, want the escape dropped rather than appended", got)
	}
}

func TestTheCursorBelongsToWhoeverDrawsIt(t *testing.T) {
	s := NewScreen(20, 5)
	// A widget speaks in its own coordinates; nobody in between carries the answer.
	out := flush(t, s, Cursor{}, func(v View) {
		v.Sub(Rect(4, 2, 10, 1)).PlaceCursor(3, 0)
	})
	if !strings.Contains(out, "\x1b[3;8H") {
		t.Fatalf("frame = %q, want the cursor at row 3 column 8", out)
	}
	if !strings.Contains(out, showCursor) {
		t.Fatalf("frame = %q, want the cursor shown", out)
	}
}

func TestAFrameNobodyPlacedTheCursorInHasNoCursor(t *testing.T) {
	s := NewScreen(8, 1)
	out := flush(t, s, Cursor{}, func(v View) { v.Text(0, 0, "text", Style{}) })
	if !strings.Contains(out, hideCursor) {
		t.Fatalf("frame = %q, want the cursor hidden when nothing owns it", out)
	}
}

func TestAWidgetScrolledOffScreenCannotMoveTheCursor(t *testing.T) {
	s := NewScreen(10, 2)
	out := flush(t, s, Cursor{}, func(v View) {
		// The box starts past the right edge, so it has nowhere to draw and no say
		// over the cursor either.
		v.Sub(Rect(20, 0, 5, 1)).PlaceCursor(0, 0)
	})
	if strings.Contains(out, showCursor) {
		t.Fatalf("frame = %q, want no cursor from a view with nowhere to draw", out)
	}
}

func TestPlacingTheCursorOnAPlainSurfaceIsHarmless(t *testing.T) {
	// A scratch surface is not a frame. Placing a cursor there means nothing, and
	// meaning nothing is not the same as being an error.
	NewSurface(4, 1).View().PlaceCursor(1, 0)
}
