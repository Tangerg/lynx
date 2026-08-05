package text

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/tui/primitives/grid"
)

// rows renders wrapped rows as plain strings, one per row, so a test can state
// the shape of the wrap.
func rows(wrapped []Wrapped) []string {
	out := make([]string, 0, len(wrapped))
	for _, w := range wrapped {
		out = append(out, w.Line.String())
	}
	return out
}

func equal(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("rows = %q, want %q", got, want)
	}
}

func TestWidthCountsColumnsNotBytes(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"中文", 4},
		{"é", 1},
		{"a\tb", 9},     // tab to the next stop at 8, then one column
		{"\t", 8},       // a tab from column zero fills the whole stop
		{"ab\tc", 9},    // two columns, tab to 8, one column
		{"\x1b[31m", 4}, // the escape byte is dropped; what follows is literal text
	} {
		if got := Width(tc.in); got != tc.want {
			t.Errorf("Width(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestWrapAtWordBoundaries(t *testing.T) {
	line := Of("the quick brown fox", grid.Style{})
	equal(t, rows(line.Wrap(10)), []string{"the quick", "brown fox"})
}

func TestWrapConsumesTheSpacesAtABreak(t *testing.T) {
	line := Of("aaa   bbb", grid.Style{})
	got := rows(line.Wrap(4))
	equal(t, got, []string{"aaa", "bbb"})
}

func TestWrapMarksContinuationRows(t *testing.T) {
	wrapped := Of("one two three", grid.Style{}).Wrap(5)
	if wrapped[0].Joined {
		t.Error("the first row claims to continue something")
	}
	for i, w := range wrapped[1:] {
		if !w.Joined {
			t.Errorf("row %d does not know it is a continuation", i+1)
		}
	}
}

func TestWrapHardBreaksAWordLongerThanTheWidth(t *testing.T) {
	line := Of("abcdefghij", grid.Style{})
	equal(t, rows(line.Wrap(4)), []string{"abcd", "efgh", "ij"})
}

func TestWrapNeverSplitsAWideCluster(t *testing.T) {
	// Three columns hold the letter and one wide cluster exactly.
	equal(t, rows(Of("a中文", grid.Style{}).Wrap(3)), []string{"a中", "文"})
	// Two cannot, so the row is left a column short rather than cutting a cluster
	// in half.
	equal(t, rows(Of("a中文", grid.Style{}).Wrap(2)), []string{"a", "中", "文"})
	// At width one nothing can hold it, so it gets a row of its own and overflows.
	equal(t, rows(Of("中", grid.Style{}).Wrap(1)), []string{"中"})
}

func TestWrapKeepsStylesAcrossBreaks(t *testing.T) {
	red := grid.Style{FG: grid.RGBColor(255, 0, 0)}
	line := Line{{Text: "hello ", Style: grid.Style{}}, {Text: "world", Style: red}}
	wrapped := line.Wrap(5)
	equal(t, rows(wrapped), []string{"hello", "world"})
	if got := wrapped[1].Line[0].Style; got != red {
		t.Fatalf("continuation style = %+v, want the span's own", got)
	}
}

func TestWrapMergesNeighbouringSpansOfOneStyle(t *testing.T) {
	line := Line{{Text: "ab"}, {Text: "cd"}}
	wrapped := line.Wrap(10)
	if len(wrapped[0].Line) != 1 {
		t.Fatalf("row has %d spans, want them merged into one", len(wrapped[0].Line))
	}
}

func TestWrapWithNoWidthReturnsTheLineWhole(t *testing.T) {
	line := Of("some text", grid.Style{})
	equal(t, rows(line.Wrap(0)), []string{"some text"})
}

func TestWrapAnEmptyLineIsOneEmptyRow(t *testing.T) {
	if got := Of("", grid.Style{}).Wrap(10); len(got) != 1 || got[0].Line.String() != "" {
		t.Fatalf("wrap of an empty line = %q, want one empty row", rows(got))
	}
}

func TestWrapAllStartsEachLineOnItsOwnRow(t *testing.T) {
	lines := []Line{Of("one", grid.Style{}), Of("", grid.Style{}), Of("two", grid.Style{})}
	equal(t, rows(WrapAll(lines, 10)), []string{"one", "", "two"})
}

func TestWrapExpandsTabs(t *testing.T) {
	// A leading tab is eight columns, so only the first word fits in twelve.
	equal(t, rows(Of("\tfunc main", grid.Style{}).Wrap(12)), []string{"        func", "main"})
}

func TestTruncate(t *testing.T) {
	for _, tc := range []struct {
		in    string
		width int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello w…"},
		{"hello", 1, "…"},
		{"hello", 0, ""},
		{"中文中文", 5, "中文…"},
	} {
		if got := Truncate(tc.in, tc.width, "…"); got != tc.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tc.in, tc.width, got, tc.want)
		}
	}
}

func TestTruncateLineKeepsStylesAndDressesTheEllipsis(t *testing.T) {
	red := grid.Style{FG: grid.RGBColor(255, 0, 0)}
	line := Line{{Text: "keep "}, {Text: "drop this", Style: red}}
	got := line.Truncate(8, "…")

	if text := got.String(); text != "keep dr…" {
		t.Fatalf("truncated = %q", text)
	}
	// The ellipsis belongs to the sentence it ends, so it wears the style of the
	// last text that survived.
	if last := got[len(got)-1]; last.Style != red || !strings.HasSuffix(last.Text, "…") {
		t.Fatalf("last span = %+v, want the ellipsis in the surviving style", last)
	}
}

func TestTruncateLineNeverSplitsAWideCluster(t *testing.T) {
	// Four columns, one for the ellipsis: the second wide cluster cannot half-fit.
	got := Of("中文中", grid.Style{}).Truncate(4, "…")
	if text := got.String(); text != "中…" {
		t.Fatalf("truncated = %q, want the wide cluster left out whole", text)
	}
}

func TestDrawPlacesTextOnTheView(t *testing.T) {
	s := grid.NewSurface(12, 1)
	red := grid.Style{FG: grid.RGBColor(255, 0, 0)}
	line := Line{{Text: "ab"}, {Text: "cd", Style: red}}

	if got := line.Draw(s.View(), 1, 0); got != 4 {
		t.Fatalf("advance = %d, want 4", got)
	}
	if got := s.CellAt(1, 0).Content; got != "a" {
		t.Fatalf("cell 1 = %q", got)
	}
	if got := s.CellAt(3, 0); got.Content != "c" || got.Style != red {
		t.Fatalf("cell 3 = %+v, want the styled span", got)
	}
}

func TestDrawExpandsTabsIntoColumns(t *testing.T) {
	s := grid.NewSurface(16, 1)
	line := Of("a\tb", grid.Style{})
	if got := line.Draw(s.View(), 0, 0); got != 9 {
		t.Fatalf("advance = %d, want 9", got)
	}
	if got := s.CellAt(0, 0).Content; got != "a" {
		t.Fatalf("cell 0 = %q", got)
	}
	// The tab is a gap, not a byte: the next letter lands on the tab stop.
	for x := 1; x < 8; x++ {
		if c := s.CellAt(x, 0); !c.Blank() {
			t.Fatalf("cell %d = %+v, want the tab to have left it blank", x, c)
		}
	}
	if got := s.CellAt(8, 0).Content; got != "b" {
		t.Fatalf("cell 8 = %q, want the letter on the tab stop", got)
	}
}

func TestControlCharactersAreDroppedRatherThanLaidOut(t *testing.T) {
	// Tool output carries escapes and carriage returns. They have no width to lay
	// out, and a cell holding one would replay it at the terminal.
	line := Of("a\x1b[31mb\rc", grid.Style{})
	got := line.Wrap(20)
	if text := got[0].Line.String(); strings.ContainsAny(text, "\x1b\r") {
		t.Fatalf("row = %q, want the control characters gone", text)
	}
}

func TestOfEmptyTextIsNoSpans(t *testing.T) {
	if got := Of("", grid.Style{}); got != nil {
		t.Fatalf("Of(\"\") = %+v, want no spans", got)
	}
}
