package atoms

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/input"
)

func wheel(action input.MouseAction) input.Event {
	return input.Mouse{Action: action}
}

func TestAFreshScrollShowsTheStart(t *testing.T) {
	// Which is what a list of items wants. Following is asked for, not assumed.
	var s Scroll
	s.Layout(10, 5)
	if s.AtBottom() || s.Offset() != 0 {
		t.Fatalf("offset = %d, following = %v, want the start", s.Offset(), s.AtBottom())
	}
}

func TestAFollowingScrollStaysAtTheEndAsContentArrives(t *testing.T) {
	var s Scroll
	s.Layout(10, 5)
	s.ToBottom()
	if got := s.Offset(); got != 5 {
		t.Fatalf("offset = %d, want the last five rows shown", got)
	}
	s.Layout(20, 5)
	if got := s.Offset(); got != 15 {
		t.Fatalf("offset = %d, want to still be showing the end", got)
	}
}

func TestScrollingUpKeepsThePlaceAsContentArrives(t *testing.T) {
	var s Scroll
	s.Layout(100, 10)
	s.ToBottom()
	s.By(-20)
	before := s.Offset()
	// Ten more rows arrive while the reader is looking at something further up.
	s.Layout(110, 10)
	if got := s.Offset(); got != before {
		t.Fatalf("offset moved from %d to %d as content arrived", before, got)
	}
	if s.AtBottom() {
		t.Fatal("scrolled up but still claims to be following the end")
	}
}

func TestScrollClampsToTheContent(t *testing.T) {
	var s Scroll
	s.Layout(10, 5)
	s.ToBottom()
	s.By(-1000)
	if got := s.Offset(); got != 0 {
		t.Fatalf("offset = %d, want the start", got)
	}
	s.By(1000)
	if !s.AtBottom() || s.Offset() != 5 {
		t.Fatalf("offset = %d, want the end", s.Offset())
	}
	// Content that shrank under a scrolled window must not leave it out of bounds.
	s.By(-3)
	s.Layout(6, 5)
	if got := s.Offset(); got > 1 {
		t.Fatalf("offset = %d, want it clamped to the smaller content", got)
	}
}

func TestScrollEverythingFitsMeansNoOffset(t *testing.T) {
	var s Scroll
	s.Layout(3, 10)
	if got := s.Offset(); got != 0 {
		t.Fatalf("offset = %d, want nothing hidden", got)
	}
	s.By(-5)
	if got := s.Offset(); got != 0 {
		t.Fatalf("offset = %d after scrolling content that fits", got)
	}
}

func TestScrollPagesKeepOneRowOfOverlap(t *testing.T) {
	var s Scroll
	s.Layout(100, 10)
	s.ToTop()
	s.Pages(1)
	// Nine rows, not ten: the reader needs one row they recognise on the other side
	// of the jump.
	if got := s.Offset(); got != 9 {
		t.Fatalf("offset after a page = %d, want 9", got)
	}
}

func TestScrollHandlesKeysAndTheWheel(t *testing.T) {
	keys := DefaultScrollKeys()
	var s Scroll
	s.Layout(100, 10)
	s.ToTop()

	if !s.Handle(key(input.Down), keys) || s.Offset() != 1 {
		t.Fatalf("offset after one down = %d", s.Offset())
	}
	if !s.Handle(wheel(input.WheelDown), keys) || s.Offset() != 1+wheelRows {
		t.Fatalf("offset after a wheel notch = %d", s.Offset())
	}
	if !s.Handle(key(input.End), keys) || !s.AtBottom() {
		t.Fatal("End did not go to the end")
	}
	if !s.Handle(key(input.Home), keys) || s.Offset() != 0 {
		t.Fatal("Home did not go to the start")
	}
	// An event it has no use for carries on to whoever else might want it.
	if s.Handle(key(input.Enter), keys) {
		t.Fatal("the scroll swallowed a key it does nothing with")
	}
	if s.Handle(input.Mouse{Action: input.MouseDown}, keys) {
		t.Fatal("the scroll swallowed a click")
	}
}

func TestScrollRowsDrawsOnlyTheVisibleSlice(t *testing.T) {
	var s Scroll
	s.ToBottom()
	rows := paint(4, 3, func(v grid.View) {
		s.Rows(v, 10, func(row grid.View, index int) {
			row.Text(0, 0, strconv.Itoa(index), grid.Style{})
		})
	})
	// The window follows the end of the content, so the last three rows are shown.
	equalRows(t, rows, []string{"7...", "8...", "9..."})
}

func TestScrollRowsStopsAtTheEndOfShortContent(t *testing.T) {
	var s Scroll
	rows := paint(4, 4, func(v grid.View) {
		s.Rows(v, 2, func(row grid.View, index int) {
			row.Text(0, 0, strconv.Itoa(index), grid.Style{})
		})
	})
	equalRows(t, rows, []string{"0...", "1...", "....", "...."})
}

// items builds a list of numbered strings.
func items(n int) []string {
	out := make([]string, n)
	for i := range n {
		out[i] = "item" + strconv.Itoa(i)
	}
	return out
}

// newList builds a list that draws each item as its text, marking the selected one.
func newList(n int) *List[string] {
	return &List[string]{
		Items: items(n),
		Keys:  DefaultListKeys(),
		Row: func(v grid.View, item string, selected bool) {
			prefix := " "
			if selected {
				prefix = ">"
			}
			v.Text(0, 0, prefix+item, grid.Style{})
		},
	}
}

func TestListSelectionMoves(t *testing.T) {
	l := newList(5)
	if got := l.Selected(); got != 0 {
		t.Fatalf("selection starts at %d, want the first item", got)
	}
	l.Handle(key(input.Down))
	l.Handle(key(input.Down))
	if got := l.Selected(); got != 2 {
		t.Fatalf("selection = %d, want 2", got)
	}
	item, ok := l.Current()
	if !ok || item != "item2" {
		t.Fatalf("current = %q, %v", item, ok)
	}
	l.Handle(key(input.End))
	if got := l.Selected(); got != 4 {
		t.Fatalf("selection after End = %d, want the last item", got)
	}
	// Without wrapping the selection stops at the end, because in a long list
	// wrapping loses the user's place.
	l.Handle(key(input.Down))
	if got := l.Selected(); got != 4 {
		t.Fatalf("selection = %d, want it to have stayed at the end", got)
	}
}

func TestListWrapsOnlyWhenAskedTo(t *testing.T) {
	l := newList(3)
	l.Wrap = true
	l.Handle(key(input.Up))
	if got := l.Selected(); got != 2 {
		t.Fatalf("selection = %d, want it wrapped to the last item", got)
	}
	l.Handle(key(input.Down))
	if got := l.Selected(); got != 0 {
		t.Fatalf("selection = %d, want it wrapped to the first", got)
	}
}

func TestListScrollsToKeepTheSelectionVisible(t *testing.T) {
	// A selection the user cannot see is one they will act on by mistake.
	l := newList(20)
	rows := paint(10, 4, func(v grid.View) { l.Draw(v) })
	if !strings.Contains(rows[0], ">item0") {
		t.Fatalf("first frame = %v, want the selection at the top", rows)
	}
	for range 6 {
		l.Handle(key(input.Down))
	}
	rows = paint(10, 4, func(v grid.View) { l.Draw(v) })
	if !strings.Contains(rows[3], ">item6") {
		t.Fatalf("frame = %v, want the selection scrolled into the last row", rows)
	}
	l.Handle(key(input.Home))
	rows = paint(10, 4, func(v grid.View) { l.Draw(v) })
	if !strings.Contains(rows[0], ">item0") {
		t.Fatalf("frame = %v, want the view back at the top", rows)
	}
}

func TestListPageMovesByAWindow(t *testing.T) {
	l := newList(50)
	paint(10, 8, func(v grid.View) { l.Draw(v) })
	l.Handle(key(input.PageDown))
	if got := l.Selected(); got != 7 {
		t.Fatalf("selection after a page = %d, want a window's worth less one", got)
	}
}

func TestListKeepsItsPlaceWhenTheItemsAreRefreshed(t *testing.T) {
	// A list refreshed while the user is reading it must not jump.
	l := newList(10)
	l.Select(5)
	l.SetItems(items(10))
	if got := l.Selected(); got != 5 {
		t.Fatalf("selection = %d, want it kept", got)
	}
	// And when the list shrank past it, the selection lands on what is there.
	l.SetItems(items(3))
	if got := l.Selected(); got != 2 {
		t.Fatalf("selection = %d, want the last item of the shorter list", got)
	}
}

func TestListWithNothingInIt(t *testing.T) {
	l := &List[string]{Keys: DefaultListKeys()}
	if got := l.Selected(); got != -1 {
		t.Fatalf("selection = %d, want none", got)
	}
	if _, ok := l.Current(); ok {
		t.Fatal("an empty list handed out an item")
	}
	// None of this may panic.
	l.Handle(key(input.Down))
	l.Handle(key(input.End))
	paint(10, 3, func(v grid.View) { l.Draw(v) })
}

func TestListIgnoresKeysItHasNoUseFor(t *testing.T) {
	l := newList(3)
	if l.Handle(key(input.Enter)) {
		t.Fatal("the list swallowed Enter, which is its container's to interpret")
	}
}

func TestTableColumnWidthsFillTheSpaceExactly(t *testing.T) {
	// The right edge has to line up with whatever is drawn beside it.
	table := Table{Columns: []Column{{Width: 6}, {Flex: 1}, {Flex: 2}}, Gap: 1}
	widths := table.Widths(30)
	total := widths[0] + widths[1] + widths[2] + 2
	if total != 30 {
		t.Fatalf("widths %v plus gaps add up to %d, want 30", widths, total)
	}
	if widths[0] != 6 {
		t.Fatalf("fixed column = %d, want 6", widths[0])
	}
	if widths[2] <= widths[1] {
		t.Fatalf("widths %v, want the larger share wider", widths)
	}
}

func TestTableFlexibleColumnsHaveAFloor(t *testing.T) {
	table := Table{Columns: []Column{{Width: 20}, {Flex: 1, Min: 4}}, Gap: 1}
	widths := table.Widths(22)
	if widths[1] < 4 {
		t.Fatalf("widths %v, want the flexible column to keep its floor", widths)
	}
}

func TestTableDrawsHeaderAndRows(t *testing.T) {
	table := Table{
		Columns: []Column{{Title: "id", Width: 4}, {Title: "name", Flex: 1}},
		Rows:    2,
		Header:  true,
		Cell: func(row, col int) (string, grid.Style) {
			return [][]string{{"a1", "alpha"}, {"b2", "bravo"}}[row][col], grid.Style{}
		},
	}
	rows := paint(12, 3, func(v grid.View) { table.Draw(v) })
	equalRows(t, rows, []string{
		"id...name...",
		"a1...alpha..",
		"b2...bravo..",
	})
	if got := table.Height(12); got != 3 {
		t.Fatalf("height = %d, want the rows plus the header", got)
	}
}

func TestTableCellsAreTruncatedToTheirColumn(t *testing.T) {
	table := Table{
		Columns: []Column{{Width: 5}, {Flex: 1}},
		Rows:    1,
		Cell:    func(int, int) (string, grid.Style) { return "far too long", grid.Style{} },
	}
	rows := paint(12, 1, func(v grid.View) { table.Draw(v) })
	// Neither cell may spill into the other's column.
	if !strings.HasPrefix(rows[0], "far …") {
		t.Fatalf("row = %q, want the first cell truncated at its column", rows[0])
	}
}

func TestTableRowStyleBandsTheWholeRow(t *testing.T) {
	selected := grid.Style{BG: grid.RGBColor(40, 40, 40)}
	table := Table{
		Columns:  []Column{{Flex: 1}},
		Rows:     2,
		RowStyle: func(row int) grid.Style { return map[bool]grid.Style{true: selected}[row == 1] },
		Cell:     func(int, int) (string, grid.Style) { return "x", grid.Style{} },
	}
	s := grid.NewSurface(6, 2)
	table.Draw(s.View())
	for x := range 6 {
		if got := s.CellAt(x, 1).Style.BG; got != selected.BG {
			t.Fatalf("column %d of the banded row = %+v, want the row style across the whole row", x, got)
		}
	}
}
