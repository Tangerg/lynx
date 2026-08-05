package atoms

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/input"
)

// typeText sends each character of s to the editor as a keystroke.
func typeText(e *Editor, s string) {
	for _, r := range s {
		e.Handle(input.Key{Code: input.Character, Rune: r})
	}
}

func ctrlKey(r rune) input.Event {
	return input.Key{Code: input.Character, Rune: r, Mods: input.Ctrl}
}

func altKey(r rune) input.Event {
	return input.Key{Code: input.Character, Rune: r, Mods: input.Alt}
}

// cursorAt lays the editor out at a width and returns the cursor's row and column.
func cursorAt(e *Editor, w, h int) (int, int) {
	surface := grid.NewSurface(w, h)
	// A plain surface has no cursor slot, so the position is taken from the layout
	// the editor itself reports.
	e.Draw(surface.View())
	rows := e.layout.rowsFor(e.lines, w)
	row, column := rowOf(rows, e.lines, e.line, e.col)
	return row - e.scroll.Offset(), column
}

func TestEditorTyping(t *testing.T) {
	e := NewEditor()
	typeText(e, "hello")
	if got := e.Text(); got != "hello" {
		t.Fatalf("text = %q", got)
	}
	_, col := e.Cursor()
	if col != 5 {
		t.Fatalf("cursor at %d, want the end", col)
	}
}

func TestEditorRefusesControlCharacters(t *testing.T) {
	// A control character has no width and would be dropped at the cell, leaving a
	// cursor position with nothing under it.
	e := NewEditor()
	e.InsertRune('\x1b')
	e.InsertRune('\r')
	if !e.Empty() {
		t.Fatalf("text = %q, want nothing", e.Text())
	}
	e.InsertRune('\t')
	if got := e.Text(); got != "\t" {
		t.Fatalf("text = %q, want the tab kept", got)
	}
}

func TestEditorMovesByClusterNotByByteOrRune(t *testing.T) {
	e := NewEditor()
	e.Insert("a中b")
	e.MoveLineStart()
	e.MoveRight()
	if _, col := e.Cursor(); col != 1 {
		t.Fatalf("after one right the cursor is at %d, want past the letter", col)
	}
	e.MoveRight()
	// The wide character is three bytes and one cluster: one press crosses it whole.
	if _, col := e.Cursor(); col != 4 {
		t.Fatalf("after two rights the cursor is at %d, want past the wide character", col)
	}
	e.MoveLeft()
	if _, col := e.Cursor(); col != 1 {
		t.Fatalf("after moving back the cursor is at %d", col)
	}
}

func TestEditorCursorSitsOnTheColumnItsCharacterOccupies(t *testing.T) {
	e := NewEditor()
	e.Insert("中文x")
	_, column := cursorAt(e, 20, 3)
	// Two wide characters are four columns, so the cursor after the letter is at five.
	if column != 5 {
		t.Fatalf("cursor column = %d, want 5", column)
	}
}

func TestEditorNewlineSplitsTheLine(t *testing.T) {
	e := NewEditor()
	e.Insert("hello world")
	e.MoveLineStart()
	for range 5 {
		e.MoveRight()
	}
	e.Newline()
	if got := e.Text(); got != "hello\n world" {
		t.Fatalf("text = %q", got)
	}
	line, col := e.Cursor()
	if line != 1 || col != 0 {
		t.Fatalf("cursor at line %d column %d, want the start of the new line", line, col)
	}
}

func TestEditorPasteArrivesAsText(t *testing.T) {
	// The point of a bracketed paste: what was pasted goes in as it was, newlines and
	// all, rather than being interpreted a keystroke at a time.
	e := NewEditor()
	e.Handle(input.Paste{Text: "func main() {\n\tprintln(1)\n}"})
	if got := e.Text(); got != "func main() {\n\tprintln(1)\n}" {
		t.Fatalf("text = %q", got)
	}
	line, col := e.Cursor()
	if line != 2 || col != 1 {
		t.Fatalf("cursor at line %d column %d, want the end of the paste", line, col)
	}
}

func TestEditorBackspaceJoinsLines(t *testing.T) {
	e := NewEditor()
	e.Insert("one\ntwo")
	e.MoveLineStart()
	e.DeleteBack()
	if got := e.Text(); got != "onetwo" {
		t.Fatalf("text = %q", got)
	}
	if _, col := e.Cursor(); col != 3 {
		t.Fatalf("cursor at %d, want where the join happened", col)
	}
}

func TestEditorBackspaceAtTheVeryStartDoesNothing(t *testing.T) {
	e := NewEditor()
	e.Insert("x")
	e.MoveLineStart()
	e.DeleteBack()
	if got := e.Text(); got != "x" {
		t.Fatalf("text = %q, want it untouched", got)
	}
}

func TestEditorDeleteForwardJoinsTheLineBelow(t *testing.T) {
	e := NewEditor()
	e.Insert("one\ntwo")
	e.MoveLineStart()
	e.MoveUp()
	e.MoveLineEnd()
	e.DeleteForward()
	if got := e.Text(); got != "onetwo" {
		t.Fatalf("text = %q", got)
	}
}

func TestEditorWordMotions(t *testing.T) {
	e := NewEditor()
	e.Insert("alpha beta_two  gamma")
	e.MoveLineEnd()

	e.MoveWordLeft()
	if _, col := e.Cursor(); col != len("alpha beta_two  ") {
		t.Fatalf("cursor at %d, want the start of the last word", col)
	}
	e.MoveWordLeft()
	// The underscore is part of a word, so a word motion in code stops where a reader
	// expects rather than in the middle of an identifier.
	if _, col := e.Cursor(); col != len("alpha ") {
		t.Fatalf("cursor at %d, want the start of the identifier", col)
	}
	e.MoveWordRight()
	if _, col := e.Cursor(); col != len("alpha beta_two") {
		t.Fatalf("cursor at %d, want past the identifier", col)
	}
}

func TestEditorDeleteWordBack(t *testing.T) {
	e := NewEditor()
	e.Insert("one two three")
	e.Handle(ctrlKey('w'))
	if got := e.Text(); got != "one two " {
		t.Fatalf("text = %q", got)
	}
	// What it removed is kept, so it can be put back.
	e.Handle(ctrlKey('y'))
	if got := e.Text(); got != "one two three" {
		t.Fatalf("text after putting it back = %q", got)
	}
}

func TestEditorKillToEndAndBack(t *testing.T) {
	e := NewEditor()
	e.Insert("keep this away")
	e.MoveLineStart()
	for range len("keep this ") {
		e.MoveRight()
	}
	e.Handle(ctrlKey('k'))
	if got := e.Text(); got != "keep this " {
		t.Fatalf("text = %q", got)
	}
	e.Handle(ctrlKey('y'))
	if got := e.Text(); got != "keep this away" {
		t.Fatalf("text after putting it back = %q", got)
	}
}

func TestEditorKillToEndSwallowsTheLineBreakWhenThereIsNothingElse(t *testing.T) {
	// What makes repeated presses take a paragraph rather than stop at the first line.
	e := NewEditor()
	e.Insert("one\ntwo")
	e.MoveLineStart()
	e.MoveUp()
	e.MoveLineEnd()
	e.Handle(ctrlKey('k'))
	if got := e.Text(); got != "onetwo" {
		t.Fatalf("text = %q", got)
	}
}

func TestEditorKillToStart(t *testing.T) {
	e := NewEditor()
	e.Insert("drop this keep")
	e.MoveLineStart()
	for range len("drop this ") {
		e.MoveRight()
	}
	e.Handle(ctrlKey('u'))
	if got := e.Text(); got != "keep" {
		t.Fatalf("text = %q", got)
	}
	if _, col := e.Cursor(); col != 0 {
		t.Fatalf("cursor at %d, want the start", col)
	}
}

func TestEditorUndoStepsOverAPhraseNotALetter(t *testing.T) {
	// One undo per keystroke would make undo useless in a composer.
	e := NewEditor()
	typeText(e, "hello world")
	e.Undo()
	if !e.Empty() {
		t.Fatalf("text after one undo = %q, want the whole phrase gone", e.Text())
	}
}

func TestEditorUndoAndRedo(t *testing.T) {
	e := NewEditor()
	typeText(e, "first")
	e.Newline()
	typeText(e, "second")

	e.Undo()
	if got := e.Text(); got != "first\n" {
		t.Fatalf("after one undo = %q", got)
	}
	e.Undo()
	if got := e.Text(); got != "first" {
		t.Fatalf("after two undos = %q", got)
	}
	e.Redo()
	if got := e.Text(); got != "first\n" {
		t.Fatalf("after redo = %q", got)
	}
	// A change after an undo abandons the redo history, which is what every editor
	// does and what stops a redo from resurrecting text the user has moved past.
	typeText(e, "third")
	e.Redo()
	if got := e.Text(); got != "first\nthird" {
		t.Fatalf("redo after a new change = %q, want it to have done nothing", got)
	}
}

func TestEditorUndoHistoryIsBounded(t *testing.T) {
	// An unbounded history in a long-lived process is a leak with a friendly name.
	e := NewEditor()
	for i := range maxUndo + 50 {
		e.Insert("x")
		e.MoveLeft()
		_ = i
	}
	if len(e.undo) > maxUndo {
		t.Fatalf("history holds %d steps, want at most %d", len(e.undo), maxUndo)
	}
}

func TestEditorVerticalMovementFollowsTheScreenNotTheParagraph(t *testing.T) {
	// A field that wraps has to move down the screen. Jumping to the next paragraph
	// would move the cursor somewhere the user cannot see the reason for.
	e := NewEditor()
	e.Insert("aaa bbb ccc ddd")
	if got := e.Height(8); got != 2 {
		t.Fatalf("height at width 8 = %d, want the text wrapped onto 2 rows", got)
	}
	cursorAt(e, 8, 4)
	e.MoveLineStart()
	cursorAt(e, 8, 4)

	e.MoveDown()
	row, _ := cursorAt(e, 8, 4)
	if row != 1 {
		t.Fatalf("after moving down the cursor is on row %d, want the second row", row)
	}
	line, _ := e.Cursor()
	if line != 0 {
		t.Fatalf("cursor moved to logical line %d, want it still inside the one paragraph", line)
	}
}

func TestEditorVerticalMovementKeepsItsColumnThroughShortLines(t *testing.T) {
	// Travelling down through a short line and out the other side has to come back to
	// the column it went in at, or the cursor drifts left and stays there.
	e := NewEditor()
	e.Insert("aaaaaaaa\nbb\ncccccccc")
	e.MoveLineStart()
	e.line, e.col = 0, 6
	cursorAt(e, 20, 5)

	e.MoveDown()
	if _, col := e.Cursor(); col != 2 {
		t.Fatalf("on the short line the cursor is at %d, want its end", col)
	}
	e.MoveDown()
	if _, col := e.Cursor(); col != 6 {
		t.Fatalf("back on a long line the cursor is at %d, want the column it started from", col)
	}
}

func TestEditorVerticalMovementStopsAtTheEnds(t *testing.T) {
	e := NewEditor()
	e.Insert("one\ntwo")
	cursorAt(e, 20, 5)
	e.MoveUp()
	e.MoveUp()
	if line, _ := e.Cursor(); line != 0 {
		t.Fatalf("cursor at line %d, want the first", line)
	}
	e.MoveDown()
	e.MoveDown()
	e.MoveDown()
	if line, _ := e.Cursor(); line != 1 {
		t.Fatalf("cursor at line %d, want the last", line)
	}
}

func TestEditorHeightFollowsTheWidthAndItsCap(t *testing.T) {
	e := NewEditor()
	e.Insert("one two three four five six seven")
	wide := e.Height(40)
	narrow := e.Height(10)
	if narrow <= wide {
		t.Fatalf("height at 10 = %d, at 40 = %d, want narrower to be taller", narrow, wide)
	}
	e.MaxRows = 2
	if got := e.Height(10); got != 2 {
		t.Fatalf("height with a cap of 2 = %d", got)
	}
}

func TestEditorScrollsToKeepTheCursorVisible(t *testing.T) {
	// A field that jumped to the end would lose the line the user is typing on.
	e := NewEditor()
	e.Insert("l1\nl2\nl3\nl4\nl5\nl6")
	row, _ := cursorAt(e, 20, 3)
	if row < 0 || row > 2 {
		t.Fatalf("cursor is on visible row %d of a 3-row field", row)
	}
	e.line, e.col = 0, 0
	row, _ = cursorAt(e, 20, 3)
	if row != 0 {
		t.Fatalf("after moving to the top the cursor is on row %d", row)
	}
}

func TestEditorPlaceholderIsNotText(t *testing.T) {
	e := NewEditor()
	e.Placeholder = "Ask anything"
	rows := paint(20, 1, func(v grid.View) { e.Draw(v) })
	if !strings.Contains(rows[0], "Ask anything") {
		t.Fatalf("row = %q, want the placeholder", rows[0])
	}
	if !e.Empty() || e.Text() != "" {
		t.Fatalf("text = %q, want the placeholder to be no part of it", e.Text())
	}
	typeText(e, "hi")
	rows = paint(20, 1, func(v grid.View) { e.Draw(v) })
	if strings.Contains(rows[0], "Ask") {
		t.Fatalf("row = %q, want the placeholder gone once there is text", rows[0])
	}
}

func TestEditorLeavesEnterToItsContainer(t *testing.T) {
	// Whether Enter sends or breaks the line is the container's decision. An editor
	// that swallowed it would take that decision away from every container.
	e := NewEditor()
	if e.Handle(input.Key{Code: input.Enter}) {
		t.Fatal("the editor consumed Enter")
	}
	if !e.Empty() {
		t.Fatalf("text = %q, want Enter to have done nothing", e.Text())
	}
	// Alt+Enter is the editor's own way to break a line, which leaves plain Enter free.
	if !e.Handle(input.Key{Code: input.Enter, Mods: input.Alt}) {
		t.Fatal("the editor ignored its newline binding")
	}
	if got := e.Text(); got != "\n" {
		t.Fatalf("text = %q", got)
	}
}

func TestEditorLeavesChordsItHasNoUseForAlone(t *testing.T) {
	e := NewEditor()
	for _, ev := range []input.Event{
		ctrlKey('g'),
		ctrlKey('c'),
		altKey('x'),
		input.Key{Code: input.F5},
		input.Key{Code: input.Character, Rune: 'a', Transition: input.Release},
	} {
		if e.Handle(ev) {
			t.Fatalf("the editor consumed %+v", ev)
		}
	}
	if !e.Empty() {
		t.Fatalf("text = %q, want nothing typed", e.Text())
	}
}

func TestEditorAcceptsShiftedCharacters(t *testing.T) {
	e := NewEditor()
	e.Handle(input.Key{Code: input.Character, Rune: 'A', Mods: input.Shift})
	if got := e.Text(); got != "A" {
		t.Fatalf("text = %q", got)
	}
}

func TestEditorPrefersTheTextTheTerminalReported(t *testing.T) {
	// The key's own code is the unshifted key on the physical keyboard. On a layout
	// where the key beside "1" produces "@", inserting the code would type "2", so
	// what the terminal says the key produced has to win.
	e := NewEditor()
	e.Handle(input.Key{Code: input.Character, Rune: '2', Text: "@"})
	if got := e.Text(); got != "@" {
		t.Fatalf("text = %q, want what the terminal reported", got)
	}
	e.Clear()
	e.Handle(input.Key{Code: input.Character, Text: "中文"})
	if got := e.Text(); got != "中文" {
		t.Fatalf("text = %q, want the reported text", got)
	}
}

func TestTheZeroEditorIsUsable(t *testing.T) {
	var e Editor
	if !e.Empty() {
		t.Fatal("the zero editor is not empty")
	}
	e.Insert("x")
	if got := e.Text(); got != "x" {
		t.Fatalf("text = %q", got)
	}
	// None of this may panic on a field nobody configured.
	e.MoveUp()
	e.MoveDown()
	e.DeleteBack()
	e.Handle(input.Key{Code: input.Left})
	paint(10, 2, func(v grid.View) { e.Draw(v) })
}

func TestEditorTextAndDrawnRowsAgree(t *testing.T) {
	// The invariant the cursor rests on: what the layout says is what is drawn.
	e := NewEditor()
	e.Insert("alpha beta gamma delta")
	const width = 12
	rows := paint(width, e.Height(width), func(v grid.View) { e.Draw(v) })
	joined := strings.Join(rows, "")
	for _, word := range []string{"alpha", "beta", "gamma", "delta"} {
		if !strings.Contains(joined, word) {
			t.Fatalf("drawn rows %v lost %q", rows, word)
		}
	}
}
