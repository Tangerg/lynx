package grid

import (
	"bytes"
	"errors"
	"image"
	"strings"
	"testing"
)

// inline renders one inline frame and returns the bytes, without the frame markers.
func inline(t *testing.T, i *Inline, cursor Cursor, draw func(View)) string {
	t.Helper()
	v := i.Frame()
	if cursor.Visible {
		v.PlaceCursor(cursor.Pos.X, cursor.Pos.Y)
	}
	if draw != nil {
		draw(v)
	}
	var buf bytes.Buffer
	if err := i.Flush(&buf); err != nil {
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

// lines draws one string per row.
func lines(rows ...string) func(View) {
	return func(v View) {
		for y, row := range rows {
			v.Text(0, y, row, Style{})
		}
	}
}

func TestAnInlineBlockIsAsTallAsWhatWasDrawn(t *testing.T) {
	// Nothing declares a height. An interface that draws two rows occupies two rows,
	// and the eight it was offered stay the terminal's.
	i := NewInline(10, 10)
	got := inline(t, i, Cursor{}, lines("ab", "cd"))
	want := sgrReset + "\r" + "ab" + eraseLine + "\r\n" + "cd" + eraseLine + "\r" + hideCursor
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
}

func TestAnInlineFrameNeverAddressesTheTerminalAbsolutely(t *testing.T) {
	// The whole model in one assertion: the block's position is decided by whatever is
	// above it, which this type does not own and cannot ask about. A single absolute
	// move would be right until the first time something else scrolled the terminal.
	i := NewInline(10, 6)
	var all strings.Builder
	all.WriteString(inline(t, i, Cursor{}, lines("one")))
	all.WriteString(inline(t, i, Cursor{}, lines("one", "two", "three")))
	i.Print(1, func(v View) { v.Text(0, 0, "done", Style{}) })
	all.WriteString(inline(t, i, Cursor{Visible: true}, lines("one", "two")))
	all.WriteString(inline(t, i, Cursor{}, lines("one")))
	all.WriteString(inline(t, i, Cursor{}, nil))

	forbidden := map[byte]string{
		'H': "absolute cursor position",
		'f': "absolute cursor position",
		'J': "erase display",
		'r': "a scrolling region",
		'S': "scroll up",
		'T': "scroll down",
	}
	for _, final := range csiFinals(all.String()) {
		if what, bad := forbidden[final]; bad {
			t.Errorf("an inline frame used %s (CSI %c), which assumes it knows where it is",
				what, final)
		}
	}
}

// csiFinals is the final byte of every CSI sequence in s, which is what says what the
// sequence did.
func csiFinals(s string) []byte {
	var out []byte
	for i := 0; i+1 < len(s); i++ {
		if s[i] != 0x1b || s[i+1] != '[' {
			continue
		}
		j := i + 2
		for j < len(s) && s[j] >= 0x20 && s[j] <= 0x3f {
			j++
		}
		if j < len(s) {
			out = append(out, s[j])
		}
		i = j
	}
	return out
}

func TestAnIdleInlineFrameIsSilent(t *testing.T) {
	// Same reason a screen's is: bytes for nothing, and every cursor command restarts
	// the terminal's blink timer.
	i := NewInline(10, 4)
	inline(t, i, Cursor{}, lines("ab"))
	if got := inline(t, i, Cursor{}, lines("ab")); got != "" {
		t.Fatalf("an unchanged frame wrote %q", got)
	}
}

func TestOnlyTheRowsThatChangedAreRewritten(t *testing.T) {
	i := NewInline(10, 4)
	inline(t, i, Cursor{}, lines("keep", "change"))

	got := inline(t, i, Cursor{}, lines("keep", "changed"))
	want := sgrReset + "\r" + "\x1b[1A" + "\r\n" + "changed" + eraseLine + "\r"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
	if strings.Contains(got, "keep") {
		t.Error("a row that did not change was rewritten")
	}
}

func TestEveryFrameStartsFromTheTopOfTheBlock(t *testing.T) {
	// The anchor is where the last frame left the cursor. Getting the count wrong by
	// one would draw the interface over the session's output, one row at a time.
	i := NewInline(10, 6)
	inline(t, i, Cursor{}, lines("a", "b", "c", "d"))
	got := inline(t, i, Cursor{}, lines("a", "b", "c", "D"))
	if !strings.HasPrefix(got, sgrReset+"\r"+"\x1b[3A") {
		t.Fatalf("frame = %q, want it to climb the three rows back to the block's top", got)
	}
}

func TestABlockThatGotShorterErasesTheRowsItGaveUp(t *testing.T) {
	// Erased where they are rather than deleted: that leaves the block shorter with
	// blank rows below it and moves nothing that is above it.
	i := NewInline(10, 4)
	inline(t, i, Cursor{}, lines("ab", "cd"))

	got := inline(t, i, Cursor{}, lines("ab"))
	want := sgrReset + "\r" + "\x1b[1A" + "\r\n" + eraseLine + "\x1b[1A"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
	// And the anchor has to come back with it, or the next frame climbs too far.
	if got := inline(t, i, Cursor{}, lines("AB")); !strings.HasPrefix(got, sgrReset+"\rAB") {
		t.Fatalf("the frame after a shrink = %q, want it to start at the block's one row", got)
	}
}

func TestABlockThatEmptiedGivesUpEveryRow(t *testing.T) {
	i := NewInline(10, 4)
	inline(t, i, Cursor{}, lines("ab", "cd"))
	got := inline(t, i, Cursor{}, nil)
	want := sgrReset + "\r" + "\x1b[1A" + eraseLine + "\r\n" + eraseLine + "\x1b[1A"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
}

func TestABlockIsTallEnoughToHoldTheCursor(t *testing.T) {
	// A caret on the row below the last thing drawn — an empty last line of an editor —
	// is still part of the interface. A block that stopped short of it would put the
	// cursor on a row it does not own.
	i := NewInline(10, 6)
	got := inline(t, i, Cursor{Visible: true, Pos: image.Pt(0, 2)}, lines("ab"))
	want := sgrReset + "\r" + "ab" + eraseLine +
		"\r\n" + eraseLine + "\r\n" + eraseLine + showCursor
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
}

func TestTheCursorIsPlacedRelativeToTheBlock(t *testing.T) {
	i := NewInline(10, 6)
	got := inline(t, i, Cursor{Visible: true, Pos: image.Pt(2, 1)}, lines("ab", "cdef"))
	if !strings.HasSuffix(got, "\r"+"\x1b[2C"+showCursor) {
		t.Fatalf("frame = %q, want the caret placed by moving across the block's last row", got)
	}
}

func TestAnUnmovedCursorIsNotRestated(t *testing.T) {
	// Every positioning command restarts the blink timer, so a caret that sits still
	// must be left alone or it never blinks.
	i := NewInline(10, 6)
	at := Cursor{Visible: true, Pos: image.Pt(2, 0)}
	inline(t, i, at, lines("abcd"))
	if got := inline(t, i, at, lines("abcd")); got != "" {
		t.Fatalf("a still caret wrote %q", got)
	}
	got := inline(t, i, Cursor{Visible: true, Pos: image.Pt(3, 0)}, lines("abcd"))
	if got != sgrReset+"\r"+"\x1b[3C" {
		t.Fatalf("frame = %q, want just the move", got)
	}
}

func TestWritingCellsReAnchorsTheCursor(t *testing.T) {
	// Writing a row leaves the terminal's cursor after the last glyph, so a caret that
	// has not moved still has to be stated again.
	i := NewInline(10, 6)
	at := Cursor{Visible: true, Pos: image.Pt(1, 0)}
	inline(t, i, at, lines("ab"))
	got := inline(t, i, at, lines("xy"))
	want := sgrReset + "\r" + "xy" + eraseLine + "\r" + "\x1b[1C"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
}

func TestPrintedRowsGoAboveTheBlock(t *testing.T) {
	// This is what "inline" is for: the interface says something final, and it becomes
	// the terminal's from then on.
	i := NewInline(10, 6)
	inline(t, i, Cursor{}, lines("prompt"))

	i.Print(1, func(v View) { v.Text(0, 0, "done", Style{}) })
	got := inline(t, i, Cursor{}, lines("prompt"))
	want := sgrReset + "\r" + "done" + eraseLine + "\r\n" + "prompt" + eraseLine + "\r"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
}

func TestPrintedRowsAreWrittenOnce(t *testing.T) {
	// They belong to the terminal now. Writing them again on the next frame would
	// print the transcript twice over.
	i := NewInline(10, 6)
	inline(t, i, Cursor{}, lines("prompt"))
	i.Print(1, func(v View) { v.Text(0, 0, "done", Style{}) })
	inline(t, i, Cursor{}, lines("prompt"))

	if got := inline(t, i, Cursor{}, lines("prompt")); got != "" {
		t.Fatalf("the frame after a print wrote %q", got)
	}
}

func TestPrintingRewritesTheBlockItPushedDown(t *testing.T) {
	// The printed rows land on the rows the block's first rows were on, and the block
	// moves down past them. Diffing against where it used to be would leave half of the
	// old interface on screen.
	i := NewInline(10, 6)
	inline(t, i, Cursor{}, lines("one", "two"))

	i.Print(1, func(v View) { v.Text(0, 0, "said", Style{}) })
	got := inline(t, i, Cursor{}, lines("one", "two"))
	if !strings.Contains(got, "one") || !strings.Contains(got, "two") {
		t.Fatalf("frame = %q, want the whole block rewritten below the printed row", got)
	}
}

func TestPrintingTakesTheRowsTheBlockNoLongerReaches(t *testing.T) {
	// A tall block that printed and then shrank leaves rows on screen below itself, and
	// they are as stale as any other row it gave up.
	i := NewInline(10, 8)
	inline(t, i, Cursor{}, lines("a", "b", "c"))

	i.Print(1, func(v View) { v.Text(0, 0, "said", Style{}) })
	got := inline(t, i, Cursor{}, lines("a"))
	want := sgrReset + "\r" + "\x1b[2A" +
		"said" + eraseLine + "\r\n" +
		"a" + eraseLine + "\r\n" + eraseLine +
		"\x1b[1A"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
}

func TestEveryPrintedRowIsPrinted(t *testing.T) {
	// Including the blank ones: a caller that laid its output out knows how tall it is,
	// and a blank row between two answers is content, not slack.
	i := NewInline(10, 4)
	i.Print(3, func(v View) {
		v.Text(0, 0, "first", Style{})
		v.Text(0, 2, "third", Style{})
	})
	got := inline(t, i, Cursor{}, nil)
	want := sgrReset + "\r" +
		"first" + eraseLine + "\r\n" +
		eraseLine + "\r\n" +
		"third" + eraseLine + "\r\n" +
		hideCursor
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
}

func TestPrintingNothingIsNotAFrame(t *testing.T) {
	i := NewInline(10, 4)
	inline(t, i, Cursor{}, lines("ab"))
	i.Print(0, func(v View) { v.Text(0, 0, "never", Style{}) })
	if got := inline(t, i, Cursor{}, lines("ab")); got != "" {
		t.Fatalf("printing no rows wrote %q", got)
	}
}

func TestPrintedRowsAreClippedToTheBlocksWidth(t *testing.T) {
	// A printed row wider than the terminal would wrap, and a wrap moves everything
	// below it — including the block, whose position is counted in rows.
	i := NewInline(6, 4)
	i.Print(1, func(v View) { v.Text(0, 0, "far too long", Style{}) })
	got := inline(t, i, Cursor{}, nil)
	if !strings.Contains(got, "far to"+eraseLine) {
		t.Fatalf("frame = %q, want the printed row clipped to six columns", got)
	}
	if strings.Contains(got, "long") {
		t.Fatalf("frame = %q, want nothing past the block's width", got)
	}
}

func TestAnInlineBlockCannotOutgrowTheRoomItWasGiven(t *testing.T) {
	// A terminal shorter than the interface is the ordinary case for a small window,
	// and a block taller than the screen has no top to climb back to.
	i := NewInline(10, 2)
	got := inline(t, i, Cursor{}, lines("one", "two", "three", "four"))
	if strings.Contains(got, "three") || strings.Contains(got, "four") {
		t.Fatalf("frame = %q, want nothing past the two rows there was room for", got)
	}
	if n := strings.Count(got, "\r\n"); n != 1 {
		t.Fatalf("frame = %q, want one row break for two rows", got)
	}
}

func TestAResizeRewritesTheWholeBlock(t *testing.T) {
	// The terminal may have reflowed what is above the block, so nothing about what it
	// is showing can be assumed.
	i := NewInline(10, 4)
	inline(t, i, Cursor{}, lines("ab", "cd"))
	i.Resize(12, 4)
	got := inline(t, i, Cursor{}, lines("ab", "cd"))
	if !strings.Contains(got, "ab") || !strings.Contains(got, "cd") {
		t.Fatalf("frame = %q, want the whole block rewritten after a resize", got)
	}
}

func TestFinishLeavesTheCursorBelowTheBlock(t *testing.T) {
	// So the shell's next prompt lands under the interface instead of on top of it.
	i := NewInline(10, 4)
	inline(t, i, Cursor{Visible: true, Pos: image.Pt(1, 0)}, lines("ab", "cd"))

	var buf bytes.Buffer
	if err := i.Finish(&buf); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	want := "\x1b[1B" + "\r\n" + sgrReset + showCursor
	if got := buf.String(); got != want {
		t.Fatalf("Finish  = %q\nwant    = %q", got, want)
	}
}

func TestFinishingAnEmptyBlockJustHandsTheCursorBack(t *testing.T) {
	// Nothing was drawn, so there is nothing to step past — and a newline for nothing
	// is a blank line the user did not ask for.
	i := NewInline(10, 4)
	var buf bytes.Buffer
	if err := i.Finish(&buf); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got := buf.String(); got != sgrReset+showCursor {
		t.Fatalf("Finish = %q", got)
	}
}

func TestAFailedInlineWriteKeepsWhatWasPrinted(t *testing.T) {
	// Output the caller asked for is worth writing twice and not worth losing.
	i := NewInline(10, 4)
	i.Print(1, func(v View) { v.Text(0, 0, "said", Style{}) })

	v := i.Frame()
	v.Text(0, 0, "prompt", Style{})
	if err := i.Flush(failing{}); err == nil {
		t.Fatal("a failed write was reported as success")
	}
	if got := inline(t, i, Cursor{}, lines("prompt")); !strings.Contains(got, "said") {
		t.Fatalf("frame = %q, want the printed row written again", got)
	}
}

// failing is a writer that never accepts anything.
type failing struct{}

func (failing) Write([]byte) (int, error) { return 0, errNope }

var errNope = errors.New("no")

func TestInlineStyleAndLinksSurviveARow(t *testing.T) {
	// Rows are printed as self-contained text, because a printed row has to keep
	// looking right with no screen behind it to restate anything.
	i := NewInline(20, 3)
	got := inline(t, i, Cursor{}, func(v View) {
		v.Text(0, 0, "plain", Style{})
		v.Text(5, 0, "loud", Style{Attr: Bold})
	})
	if !strings.Contains(got, "plain") || !strings.Contains(got, "loud") {
		t.Fatalf("frame = %q", got)
	}
	if !strings.Contains(got, "\x1b[0;1m") {
		t.Fatalf("frame = %q, want the bold run stated", got)
	}
	if !strings.Contains(got, "loud"+sgrReset) {
		t.Fatalf("frame = %q, want the row to end at the default style", got)
	}
}

func TestInlineSizeIsTheRoomItWasGiven(t *testing.T) {
	i := NewInline(30, 5)
	if w, h := i.Size(); w != 30 || h != 5 {
		t.Fatalf("Size = %d, %d", w, h)
	}
	i.Resize(20, 3)
	if w, h := i.Size(); w != 20 || h != 3 {
		t.Fatalf("Size after a resize = %d, %d", w, h)
	}
}

func TestAnInlineBlockWithNoRoomDrawsNothing(t *testing.T) {
	// None of this may panic: a layout collapses before it disappears.
	for _, size := range [][2]int{{0, 0}, {0, 4}, {4, 0}} {
		i := NewInline(size[0], size[1])
		inline(t, i, Cursor{}, lines("ab", "cd"))
		i.Print(2, func(v View) { v.Text(0, 0, "said", Style{}) })
		inline(t, i, Cursor{}, nil)
		var buf bytes.Buffer
		if err := i.Finish(&buf); err != nil {
			t.Fatalf("Finish: %v", err)
		}
	}
}

func TestInlineRowsAreEndedWithACarriageReturnBeforeTheNewline(t *testing.T) {
	// A row that filled the last column leaves the terminal in its pending-wrap state,
	// and a bare newline there advances twice — which takes the block's anchor with it.
	i := NewInline(4, 3)
	got := inline(t, i, Cursor{}, lines("abcd", "efgh"))
	if !strings.Contains(got, "abcd"+eraseLine+"\r\n") {
		t.Fatalf("frame = %q, want the row break to begin with a carriage return", got)
	}
	for at := 0; at < len(got); at++ {
		if got[at] == '\n' && (at == 0 || got[at-1] != '\r') {
			t.Fatalf("frame = %q has a newline at %d with no carriage return before it", got, at)
		}
	}
}
