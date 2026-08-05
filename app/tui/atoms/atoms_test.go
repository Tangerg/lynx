package atoms

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/input"
	"github.com/Tangerg/lynx/app/tui/primitives/text"
)

// paint draws a widget into a surface of the given size and returns what it looks
// like, one string per row with a dot for a blank cell.
func paint(w, h int, draw func(grid.View)) []string {
	s := grid.NewSurface(w, h)
	draw(s.View())
	rows := make([]string, 0, h)
	for y := range h {
		var b strings.Builder
		for x := range w {
			c := s.CellAt(x, y)
			switch {
			case c.Width() == 0:
			case c.Content == "":
				b.WriteByte('.')
			default:
				b.WriteString(c.Content)
			}
		}
		rows = append(rows, b.String())
	}
	return rows
}

func equalRows(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("drawn:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func key(code input.Code) input.Event { return input.Key{Code: code} }

func TestBoxFramesAndReportsWhatIsLeft(t *testing.T) {
	box := Box{Border: Rounded, Padding: Uniform(1)}
	rows := paint(8, 5, func(v grid.View) {
		inner := box.Draw(v)
		w, h := inner.Size()
		if w != 4 || h != 1 {
			t.Fatalf("inner = %dx%d, want 4x1", w, h)
		}
		inner.Text(0, 0, "abcd", grid.Style{})
	})
	equalRows(t, rows, []string{
		"╭──────╮",
		"│......│",
		"│.abcd.│",
		"│......│",
		"╰──────╯",
	})
}

func TestBoxOverheadMatchesWhatItDraws(t *testing.T) {
	// A box that reported one overhead and drew another would have its content
	// clipped, and the bug would look like it belonged to the content.
	for _, box := range []Box{
		{},
		{Border: Rounded},
		{Padding: Uniform(2)},
		{Border: Square, Padding: Symmetric(1, 2)},
	} {
		w, h := box.Overhead()
		s := grid.NewSurface(20, 10)
		inner := box.Inner(s.View())
		iw, ih := inner.Size()
		if iw != 20-w || ih != 10-h {
			t.Errorf("box %+v: inner %dx%d does not match overhead %dx%d", box, iw, ih, w, h)
		}
	}
}

func TestBoxTitleSitsInTheBorder(t *testing.T) {
	rows := paint(12, 2, func(v grid.View) {
		Box{Border: Rounded, Title: "Plan"}.Draw(v)
	})
	if !strings.Contains(rows[0], "Plan") {
		t.Fatalf("top border = %q, want the title in it", rows[0])
	}
	// A column of border survives on each side, or the line stops reading as a frame.
	if !strings.HasPrefix(rows[0], "╭─") || !strings.HasSuffix(rows[0], "─╮") {
		t.Fatalf("top border = %q, want border either side of the title", rows[0])
	}
}

func TestBoxSurvivesBeingSqueezed(t *testing.T) {
	// A collapsing layout must look small, not corrupted. None of these may panic.
	for _, size := range [][2]int{{0, 0}, {1, 1}, {2, 1}, {1, 3}, {3, 2}} {
		paint(size[0], size[1], func(v grid.View) {
			Box{Border: Rounded, Title: "title", Footer: "footer", Padding: Uniform(1)}.Draw(v)
		})
	}
}

func TestLabelTruncatesRatherThanWraps(t *testing.T) {
	// One row is all a label ever has. Folding would push whatever is below it off
	// the screen.
	rows := paint(6, 2, func(v grid.View) {
		Label{Text: "far too long", Ellipsis: "…"}.Draw(v)
	})
	equalRows(t, rows, []string{"far t…", "......"})
}

func TestLabelAlignment(t *testing.T) {
	for _, tc := range []struct {
		align Align
		want  string
	}{
		{Start, "ab........"},
		{Center, "....ab...."},
		{End, "........ab"},
	} {
		rows := paint(10, 1, func(v grid.View) {
			Label{Text: "ab", Align: tc.align}.Draw(v)
		})
		equalRows(t, rows, []string{tc.want})
	}
}

func TestParagraphHeightFollowsWidth(t *testing.T) {
	p := Of("one two three four", grid.Style{})
	if got := p.Height(9); got != 3 {
		t.Fatalf("height at 9 = %d, want 3", got)
	}
	if got := p.Height(4); got != 5 {
		t.Fatalf("height at 4 = %d, want 5", got)
	}
	// And what it reports is what it draws, or a container's layout is a guess.
	rows := paint(9, p.Height(9), func(v grid.View) { p.Draw(v) })
	equalRows(t, rows, []string{"one two..", "three....", "four....."})
}

func TestParagraphKeepsNewlinesAsLineBreaks(t *testing.T) {
	p := Of("first\nsecond", grid.Style{})
	if got := p.Height(20); got != 2 {
		t.Fatalf("height = %d, want a row per line", got)
	}
}

func TestParagraphIndentsEveryRow(t *testing.T) {
	p := Of("one two three", grid.Style{})
	p.Indent = 2
	rows := paint(7, 3, func(v grid.View) { p.Draw(v) })
	equalRows(t, rows, []string{"..one..", "..two..", "..three"})
}

func TestParagraphCapsItsHeight(t *testing.T) {
	p := Of("one two three four five", grid.Style{})
	p.MaxRows = 2
	if got := p.Height(6); got != 2 {
		t.Fatalf("height = %d, want the cap", got)
	}
	rows := paint(6, 2, func(v grid.View) { p.Draw(v) })
	// The last surviving row usually fits, so truncating it would say nothing. It
	// still has to tell the reader there is more.
	if !strings.Contains(rows[1], "…") {
		t.Fatalf("last row = %q, want it to say it was cut", rows[1])
	}
}

func TestParagraphRewrapsWhenItsTextChanges(t *testing.T) {
	// The wrap is memoised because it is asked for twice a frame. A memo that
	// outlived its content would show the old text forever.
	p := Of("short", grid.Style{})
	if got := p.Height(20); got != 1 {
		t.Fatalf("height = %d", got)
	}
	p.SetText(linesOf("one\ntwo\nthree", grid.Style{}))
	if got := p.Height(20); got != 3 {
		t.Fatalf("height after the text changed = %d, want 3", got)
	}
}

func TestRowsGivesEachSlotTheHeightItAsksFor(t *testing.T) {
	var drawn []int
	mark := func(n int) Widget { return widgetFunc(func(v grid.View) { drawn = append(drawn, n) }) }

	views := Rows(grid.NewSurface(10, 10).View(),
		Slot{Widget: mark(0), Size: Fixed(2)},
		Slot{Widget: mark(1), Size: Flex(1)},
		Slot{Widget: mark(2), Size: Fixed(3)},
	)
	heights := []int{}
	for _, v := range views {
		_, h := v.Size()
		heights = append(heights, h)
	}
	if heights[0] != 2 || heights[1] != 5 || heights[2] != 3 {
		t.Fatalf("heights = %v, want the fixed slots honoured and the rest flexed", heights)
	}
	if len(drawn) != 3 {
		t.Fatalf("drew %d slots, want all of them", len(drawn))
	}
}

func TestRowsMeasuresTheSlotsThatAskToBeMeasured(t *testing.T) {
	p := Of("one two three four five six", grid.Style{})
	views := Rows(grid.NewSurface(10, 10).View(),
		Slot{Widget: p, Size: Measured(0, 3)},
		Slot{Widget: nil, Size: Flex(1)},
	)
	_, measured := views[0].Size()
	if measured != 3 {
		t.Fatalf("measured slot = %d rows, want its cap of 3", measured)
	}
	_, rest := views[1].Size()
	if rest != 7 {
		t.Fatalf("flexible slot = %d rows, want the remaining 7", rest)
	}
}

func TestRowsSplitsTheRemainderWithoutLosingARow(t *testing.T) {
	// A row lost to rounding is a gap the user can see.
	views := Rows(grid.NewSurface(4, 10).View(),
		Slot{Size: Flex(1)},
		Slot{Size: Flex(2)},
	)
	total := 0
	for _, v := range views {
		_, h := v.Size()
		total += h
	}
	if total != 10 {
		t.Fatalf("slots add up to %d rows, want all 10", total)
	}
}

func TestRowsDrawsSlotsSqueezedToNothing(t *testing.T) {
	// A widget's draw code runs every frame. One that only breaks when it has no
	// room breaks in front of the user.
	drawn := false
	Rows(grid.NewSurface(10, 1).View(),
		Slot{Widget: widgetFunc(func(grid.View) {}), Size: Fixed(1)},
		Slot{Widget: widgetFunc(func(v grid.View) {
			drawn = true
			if !v.Empty() {
				t.Error("a slot with no room got somewhere to draw")
			}
		}), Size: Fixed(5)},
	)
	if !drawn {
		t.Fatal("a slot with no room was skipped instead of drawn")
	}
}

func TestColumnsLaysOutSideBySide(t *testing.T) {
	views := Columns(grid.NewSurface(10, 3).View(),
		Slot{Size: Fixed(3)},
		Slot{Size: Flex(1)},
	)
	if w, h := views[0].Size(); w != 3 || h != 3 {
		t.Fatalf("first = %dx%d, want 3x3", w, h)
	}
	if w, _ := views[1].Size(); w != 7 {
		t.Fatalf("second width = %d, want 7", w)
	}
}

func TestSpinnerAdvancesOnlyWhenTold(t *testing.T) {
	// It holds a frame number, not a clock: an idle UI must not wake up to animate
	// something nobody is waiting for.
	s := &Spinner{Frames: []string{"a", "b"}, Label: "working"}
	first := paint(10, 1, func(v grid.View) { s.Draw(v) })
	again := paint(10, 1, func(v grid.View) { s.Draw(v) })
	equalRows(t, first, again)

	s.Tick()
	next := paint(10, 1, func(v grid.View) { s.Draw(v) })
	if next[0] == first[0] {
		t.Fatalf("frame did not change after a tick: %q", next[0])
	}
	if !strings.Contains(next[0], "working") {
		t.Fatalf("row = %q, want the label", next[0])
	}
}

func TestSpinnerDropsALabelThatDoesNotFit(t *testing.T) {
	s := &Spinner{Frames: []string{"a"}, Label: "far too long to fit"}
	rows := paint(3, 1, func(v grid.View) { s.Draw(v) })
	if !strings.HasPrefix(rows[0], "a") {
		t.Fatalf("row = %q, want the glyph", rows[0])
	}
}

func TestScrollbarThumbTracksThePosition(t *testing.T) {
	top := paint(1, 4, func(v grid.View) {
		Scrollbar{Total: 40, Window: 4, Offset: 0, Track: "-", Thumb: "#"}.Draw(v)
	})
	if top[0] != "#" || top[3] != "-" {
		t.Fatalf("at the top the bar is %v, want the thumb at the top", top)
	}
	bottom := paint(1, 4, func(v grid.View) {
		Scrollbar{Total: 40, Window: 4, Offset: 36, Track: "-", Thumb: "#"}.Draw(v)
	})
	if bottom[3] != "#" || bottom[0] != "-" {
		t.Fatalf("at the end the bar is %v, want the thumb at the bottom", bottom)
	}
}

func TestScrollbarThumbNeverRoundsAway(t *testing.T) {
	// A thumb rounded down to nothing tells the user nothing.
	rows := paint(1, 4, func(v grid.View) {
		Scrollbar{Total: 10000, Window: 4, Offset: 5000, Track: "-", Thumb: "#"}.Draw(v)
	})
	if !strings.Contains(strings.Join(rows, ""), "#") {
		t.Fatalf("bar = %v, want a thumb somewhere in it", rows)
	}
}

func TestScrollbarKnowsWhenItIsPointless(t *testing.T) {
	if (Scrollbar{Total: 3, Window: 10}).Needed() {
		t.Error("a bar claims to be needed when everything already fits")
	}
	if !(Scrollbar{Total: 30, Window: 10}).Needed() {
		t.Error("a bar does not know it is needed")
	}
}

func TestHelpShowsWhatFitsAndDropsTheRest(t *testing.T) {
	help := Help{Bindings: []Binding{
		{Key: input.Key{Code: input.Enter}, Does: "send"},
		{Key: input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl}, Does: "quit"},
		{Key: input.Key{Code: input.Character, Rune: 'g', Mods: input.Ctrl}, Does: "tasks"},
	}}
	full := paint(40, 1, func(v grid.View) { help.Draw(v) })
	for _, want := range []string{"enter send", "ctrl+c quit", "ctrl+g tasks"} {
		if !strings.Contains(full[0], want) {
			t.Fatalf("row = %q, want %q in it", full[0], want)
		}
	}
	// Half a hint is not a hint, so the ones that do not fit are dropped whole and
	// the order decides which survive.
	narrow := paint(14, 1, func(v grid.View) { help.Draw(v) })
	if !strings.Contains(narrow[0], "enter send") {
		t.Fatalf("row = %q, want the first hint kept", narrow[0])
	}
	if strings.Contains(narrow[0], "ctrl+g") {
		t.Fatalf("row = %q, want the hints that did not fit dropped", narrow[0])
	}
}

func TestHelpSkipsHiddenBindings(t *testing.T) {
	help := Help{Bindings: []Binding{
		{Key: input.Key{Code: input.Enter}, Does: "send"},
		{Key: input.Key{Code: input.F5}, Does: "secret", Hidden: true},
	}}
	rows := paint(40, 1, func(v grid.View) { help.Draw(v) })
	if strings.Contains(rows[0], "secret") {
		t.Fatalf("row = %q, want the hidden binding left out", rows[0])
	}
}

func TestBindingMatchesTheKeystrokeItDescribes(t *testing.T) {
	// The hint and the handler have to be talking about the same thing, which is the
	// whole reason they are one value.
	b := Binding{Key: input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl}, Does: "save"}
	if !b.Matches(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl}) {
		t.Error("a binding does not match its own keystroke")
	}
	if b.Matches(input.Key{Code: input.Character, Rune: 's'}) {
		t.Error("a binding matched the same letter without its modifier")
	}
	if b.Matches(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl, Transition: input.Release}) {
		t.Error("a binding fired on the key coming back up")
	}
	if got := b.Key.String(); got != "ctrl+s" {
		t.Fatalf("hint text = %q", got)
	}
}

// widgetFunc adapts a function to [Widget].
type widgetFunc func(grid.View)

func (f widgetFunc) Draw(v grid.View) { f(v) }

func TestTextIsMeasuredTheSameWayItIsDrawn(t *testing.T) {
	// The one invariant the whole layer rests on: a widget that measured text one
	// way and drew it another would misalign everything beside it.
	for _, s := range []string{"abc", "中文", "a中b", "é", "tab\there"} {
		width := text.Width(s)
		rows := paint(width+3, 1, func(v grid.View) { v.Text(0, 0, s, grid.Style{}) })
		if got := strings.TrimRight(rows[0], "."); len([]rune(got)) == 0 {
			t.Fatalf("%q drew nothing", s)
		}
		if tail := rows[0][len(rows[0])-3:]; tail != "..." {
			t.Fatalf("%q measured %d columns but drew past them: %q", s, width, rows[0])
		}
	}
}
