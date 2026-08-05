package input

import (
	"image"
	"strings"
	"testing"
)

// feed decodes a whole string in one go.
func feed(s string) []Event {
	var p Parser
	return p.Feed([]byte(s))
}

// one decodes a string that must produce exactly one event.
func one(t *testing.T, s string) Event {
	t.Helper()
	events := feed(s)
	if len(events) != 1 {
		t.Fatalf("decoding %q produced %d events, want 1: %+v", s, len(events), events)
	}
	return events[0]
}

func TestPlainCharacters(t *testing.T) {
	events := feed("ab中")
	want := []Key{
		{Code: Character, Rune: 'a'},
		{Code: Character, Rune: 'b'},
		{Code: Character, Rune: '中'},
	}
	if len(events) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(events), len(want), events)
	}
	for i, w := range want {
		if got := events[i].(Key); got != w {
			t.Errorf("event %d = %+v, want %+v", i, got, w)
		}
	}
}

func TestNamedKeysAndControlChords(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Key
	}{
		{"\r", Key{Code: Enter}},
		{"\t", Key{Code: Tab}},
		{"\x7f", Key{Code: Backspace}},
		{"\x08", Key{Code: Backspace}},
		{"\x03", Key{Code: Character, Rune: 'c', Mods: Ctrl}},
		{"\x01", Key{Code: Character, Rune: 'a', Mods: Ctrl}},
		{"\x0a", Key{Code: Character, Rune: 'j', Mods: Ctrl}},
		// The C0 bytes that are not keys of their own. A decoder that dropped these
		// would make the chords unbindable, which is how Ctrl+Space stops working.
		{"\x00", Key{Code: Character, Rune: ' ', Mods: Ctrl}},
		{"\x1c", Key{Code: Character, Rune: '\\', Mods: Ctrl}},
		{"\x1d", Key{Code: Character, Rune: ']', Mods: Ctrl}},
		{"\x1e", Key{Code: Character, Rune: '^', Mods: Ctrl}},
		{"\x1f", Key{Code: Character, Rune: '_', Mods: Ctrl}},
	} {
		if got := one(t, tc.in).(Key); got != tc.want {
			t.Errorf("%q = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestAltChords(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Key
	}{
		{"\x1bx", Key{Code: Character, Rune: 'x', Mods: Alt}},
		{"\x1b中", Key{Code: Character, Rune: '中', Mods: Alt}},
		{"\x1b\r", Key{Code: Enter, Mods: Alt}},
	} {
		if got := one(t, tc.in).(Key); got != tc.want {
			t.Errorf("%q = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestCursorAndFunctionKeys(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Key
	}{
		{"\x1b[A", Key{Code: Up}},
		{"\x1b[B", Key{Code: Down}},
		{"\x1b[C", Key{Code: Right}},
		{"\x1b[D", Key{Code: Left}},
		{"\x1b[H", Key{Code: Home}},
		{"\x1b[F", Key{Code: End}},
		{"\x1b[1;5A", Key{Code: Up, Mods: Ctrl}},
		{"\x1b[1;2D", Key{Code: Left, Mods: Shift}},
		{"\x1b[1;3B", Key{Code: Down, Mods: Alt}},
		{"\x1b[1;8C", Key{Code: Right, Mods: Shift | Alt | Ctrl}},
		{"\x1b[Z", Key{Code: Backtab, Mods: Shift}},
		{"\x1b[3~", Key{Code: Delete}},
		{"\x1b[5~", Key{Code: PageUp}},
		{"\x1b[6~", Key{Code: PageDown}},
		{"\x1b[2~", Key{Code: Insert}},
		{"\x1b[1~", Key{Code: Home}},
		{"\x1b[4~", Key{Code: End}},
		{"\x1b[15~", Key{Code: F5}},
		{"\x1b[24~", Key{Code: F12}},
		{"\x1b[15;5~", Key{Code: F5, Mods: Ctrl}},
		{"\x1bOP", Key{Code: F1}},
		{"\x1bOA", Key{Code: Up}},
		{"\x1b[P", Key{Code: F1}},
		{"\x1b[S", Key{Code: F4}},
	} {
		if got := one(t, tc.in).(Key); got != tc.want {
			t.Errorf("%q = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestExtendedKeyReports(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want Key
	}{
		{"plain letter", "\x1b[97u", Key{Code: Character, Rune: 'a'}},
		{"with modifier", "\x1b[97;5u", Key{Code: Character, Rune: 'a', Mods: Ctrl}},
		{"repeat", "\x1b[97;1:2u", Key{Code: Character, Rune: 'a', Transition: Repeat}},
		{"release", "\x1b[97;1:3u", Key{Code: Character, Rune: 'a', Transition: Release}},
		{"named key", "\x1b[57352u", Key{Code: Up}},
		{"function key", "\x1b[57364u", Key{Code: F1}},
		{"super modifier", "\x1b[97;9u", Key{Code: Character, Rune: 'a', Mods: Super}},
		{"associated text", "\x1b[97;1;98u", Key{Code: Character, Rune: 'a', Text: "b"}},
		{"alternate codes accepted", "\x1b[97:65:97u", Key{Code: Character, Rune: 'a'}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := one(t, tc.in).(Key); got != tc.want {
				t.Errorf("%q = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestExtendedKeyReportsThatMustBeRefused(t *testing.T) {
	// A key event with the wrong modifiers fires something the user did not ask
	// for, so a report that cannot be trusted produces nothing at all.
	for _, in := range []string{
		"\x1b[u",               // a cursor report, not a key
		"\x1b[0u",              // no key has code zero
		"\x1b[97;99999999999u", // a modifier parameter beyond any encoding
		"\x1b[97;1:9u",         // an event type this protocol does not define
		"\x1b[97;1:2:3u",       // too many subparameters
		"\x1b[57400u",          // a private-use number this program does not know
		"\x1b[97;1;2;3u",       // more parameter groups than the report has
		"\x1b[97;1;1114112u",   // associated text outside Unicode
	} {
		if events := feed(in); len(events) != 0 {
			t.Errorf("%q produced %+v, want nothing", in, events)
		}
	}
}

func TestMouseReports(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want Mouse
	}{
		{"left press", "\x1b[<0;10;5M", Mouse{Pos: image.Pt(9, 4), Action: MouseDown, Button: ButtonLeft}},
		{"left release", "\x1b[<0;10;5m", Mouse{Pos: image.Pt(9, 4), Action: MouseUp, Button: ButtonLeft}},
		{"middle press", "\x1b[<1;1;1M", Mouse{Pos: image.Pt(0, 0), Action: MouseDown, Button: ButtonMiddle}},
		{"right press", "\x1b[<2;1;1M", Mouse{Pos: image.Pt(0, 0), Action: MouseDown, Button: ButtonRight}},
		{"drag", "\x1b[<32;3;4M", Mouse{Pos: image.Pt(2, 3), Action: MouseDrag, Button: ButtonLeft}},
		{"move", "\x1b[<35;3;4M", Mouse{Pos: image.Pt(2, 3), Action: MouseMove}},
		{"wheel up", "\x1b[<64;3;4M", Mouse{Pos: image.Pt(2, 3), Action: WheelUp}},
		{"wheel down", "\x1b[<65;3;4M", Mouse{Pos: image.Pt(2, 3), Action: WheelDown}},
		{"shift and ctrl", "\x1b[<20;1;1M", Mouse{Pos: image.Pt(0, 0), Action: MouseDown, Button: ButtonLeft, Mods: Shift | Ctrl}},
		{"every modifier", "\x1b[<28;1;1M", Mouse{Pos: image.Pt(0, 0), Action: MouseDown, Button: ButtonLeft, Mods: Shift | Alt | Ctrl}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := one(t, tc.in).(Mouse); got != tc.want {
				t.Errorf("%q = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestMouseReportsThatMustBeRefused(t *testing.T) {
	for _, in := range []string{
		"\x1b[<0;10M",         // no row
		"\x1b[<0;99999999;5M", // a column beyond any encoding
		"\x1b[<66;1;1M",       // the horizontal wheel, which nothing here reads
		"\x1b[<99999999;1;1M", // a button field beyond any encoding
	} {
		if events := feed(in); len(events) != 0 {
			t.Errorf("%q produced %+v, want nothing", in, events)
		}
	}
}

func TestFocusReports(t *testing.T) {
	if _, ok := one(t, "\x1b[I").(FocusIn); !ok {
		t.Error("CSI I did not decode as focus gained")
	}
	if _, ok := one(t, "\x1b[O").(FocusOut); !ok {
		t.Error("CSI O did not decode as focus lost")
	}
}

func TestPaste(t *testing.T) {
	ev := one(t, "\x1b[200~hello world\x1b[201~")
	paste, ok := ev.(Paste)
	if !ok {
		t.Fatalf("event = %T, want Paste", ev)
	}
	if paste.Text != "hello world" {
		t.Fatalf("pasted %q", paste.Text)
	}
}

func TestPasteKeepsWhatWouldOtherwiseBeInterpreted(t *testing.T) {
	// The whole point of a bracketed paste: what is inside is text, even when it
	// looks like keys. Interpreting it is how pasting code runs commands.
	ev := one(t, "\x1b[200~line\rnext\ttab\x1b[Anot-a-key\x1b[201~")
	paste := ev.(Paste)
	if paste.Text != "line\rnext\ttab\x1b[Anot-a-key" {
		t.Fatalf("pasted %q, want the bytes verbatim", paste.Text)
	}
}

func TestPasteSplitAcrossReads(t *testing.T) {
	var p Parser
	if events := p.Feed([]byte("\x1b[200~part one ")); len(events) != 0 {
		t.Fatalf("an unfinished paste produced %+v", events)
	}
	// Split inside the closing sequence, which is where a naive decoder loses the
	// paste or emits its terminator as keys.
	if events := p.Feed([]byte("part two\x1b[20")); len(events) != 0 {
		t.Fatalf("a split terminator produced %+v", events)
	}
	events := p.Feed([]byte("1~"))
	if len(events) != 1 {
		t.Fatalf("got %+v, want the completed paste", events)
	}
	if got := events[0].(Paste).Text; got != "part one part two" {
		t.Fatalf("pasted %q", got)
	}
}

func TestFlushDoesNotCutAPasteShort(t *testing.T) {
	var p Parser
	p.Feed([]byte("\x1b[200~half"))
	// A paste is incomplete, not ambiguous: forcing it would corrupt the text.
	if events := p.Flush(); len(events) != 0 {
		t.Fatalf("Flush produced %+v mid-paste", events)
	}
	events := p.Feed([]byte(" whole\x1b[201~"))
	if got := events[0].(Paste).Text; got != "half whole" {
		t.Fatalf("pasted %q", got)
	}
}

func TestAnUnclosedPasteIsBounded(t *testing.T) {
	// A terminal that opens a paste and never closes it would otherwise swallow
	// everything typed afterwards into a buffer that only grows.
	var p Parser
	p.Feed([]byte("\x1b[200~"))
	var events []Event
	chunk := []byte(strings.Repeat("x", 1<<16))
	for range (maxPaste / len(chunk)) + 2 {
		events = append(events, p.Feed(chunk)...)
	}
	if len(events) == 0 {
		t.Fatal("an unbounded paste never delivered anything")
	}
	if p.pasting {
		t.Fatal("still pasting after the bound was reached")
	}
}

func TestStrayPasteTerminatorIsIgnored(t *testing.T) {
	if events := feed("\x1b[201~"); len(events) != 0 {
		t.Fatalf("a terminator with no paste open produced %+v", events)
	}
}

func TestLoneEscapeResolvesOnlyOnFlush(t *testing.T) {
	var p Parser
	if events := p.Feed([]byte("\x1b")); len(events) != 0 {
		t.Fatalf("a lone escape decoded immediately as %+v", events)
	}
	if !p.Pending() {
		t.Fatal("Pending does not report the buffered escape, so no timer would be armed")
	}
	events := p.Flush()
	if len(events) != 1 || events[0].(Key).Code != Esc {
		t.Fatalf("Flush produced %+v, want the Escape key", events)
	}
	if p.Pending() {
		t.Fatal("something is still buffered after Flush")
	}
}

func TestFlushBetweenEscapeAndItsSequence(t *testing.T) {
	// The user pressed Escape and then typed a bracket. Waiting made it ambiguous;
	// the flush says the waiting is over, so it is two keystrokes.
	var p Parser
	p.Feed([]byte("\x1b["))
	events := p.Flush()
	if len(events) != 2 {
		t.Fatalf("got %+v, want the Escape key and the bracket", events)
	}
	if got := events[0].(Key).Code; got != Esc {
		t.Fatalf("first = %v, want Escape", got)
	}
	if got := events[1].(Key); !got.IsRune('[', 0) {
		t.Fatalf("second = %+v, want the bracket", got)
	}
}

func TestTwoEscapesInARow(t *testing.T) {
	events := feed("\x1b\x1b")
	if len(events) != 1 || events[0].(Key).Code != Esc {
		t.Fatalf("got %+v, want one Escape with the second still buffered", events)
	}
}

func TestSequencesSplitAtEveryBoundary(t *testing.T) {
	// A read can land anywhere. Every split of a sequence has to decode to the
	// same event as the whole, or keys are lost under load.
	const seq = "\x1b[1;5A"
	for split := 1; split < len(seq); split++ {
		var p Parser
		events := append(p.Feed([]byte(seq[:split])), p.Feed([]byte(seq[split:]))...)
		if len(events) != 1 {
			t.Fatalf("split at %d produced %+v, want one event", split, events)
		}
		if got := events[0].(Key); got != (Key{Code: Up, Mods: Ctrl}) {
			t.Fatalf("split at %d produced %+v", split, got)
		}
	}
}

func TestOneByteAtATime(t *testing.T) {
	const in = "a\x1b[B\x1b[<0;2;3M\x1b[200~p\x1b[201~"
	var p Parser
	var events []Event
	for i := range len(in) {
		events = append(events, p.Feed([]byte{in[i]})...)
	}
	events = append(events, p.Flush()...)
	if len(events) != 4 {
		t.Fatalf("got %d events, want 4: %+v", len(events), events)
	}
	if got := events[0].(Key); !got.IsRune('a', 0) {
		t.Fatalf("first = %+v", got)
	}
	if got := events[1].(Key).Code; got != Down {
		t.Fatalf("second = %v", got)
	}
	if got := events[2].(Mouse).Pos; got != image.Pt(1, 2) {
		t.Fatalf("third = %v", got)
	}
	if got := events[3].(Paste).Text; got != "p" {
		t.Fatalf("fourth = %q", got)
	}
}

func TestCharacterSplitAcrossReads(t *testing.T) {
	multi := []byte("中")
	var p Parser
	if events := p.Feed(multi[:1]); len(events) != 0 {
		t.Fatalf("half a character decoded as %+v", events)
	}
	events := p.Feed(multi[1:])
	if len(events) != 1 || events[0].(Key).Rune != '中' {
		t.Fatalf("got %+v, want the reassembled character", events)
	}
}

func TestABrokenCharacterIsDroppedWithoutTakingTheNextOneWithIt(t *testing.T) {
	var p Parser
	// The leading byte of a three-byte character, followed by something that cannot
	// continue it. The pair is already known to be invalid — it does not wait for a
	// flush — and the letter after it still has to arrive.
	events := p.Feed([]byte("中")[:1])
	if len(events) != 0 {
		t.Fatalf("a lone leading byte decoded as %+v", events)
	}
	events = p.Feed([]byte("x"))
	events = append(events, p.Flush()...)
	if len(events) != 1 {
		t.Fatalf("got %+v, want only the letter", events)
	}
	if got := events[0].(Key); !got.IsRune('x', 0) {
		t.Fatalf("got %+v, want the letter", got)
	}
}

func TestInvalidUTF8IsDropped(t *testing.T) {
	events := feed("\xffa")
	if len(events) != 1 {
		t.Fatalf("got %+v, want only the letter", events)
	}
	if got := events[0].(Key); !got.IsRune('a', 0) {
		t.Fatalf("got %+v", got)
	}
}

func TestRunawaySequenceIsDiscarded(t *testing.T) {
	// A stream that opens a sequence and never ends it must not grow the buffer
	// without limit.
	var p Parser
	p.Feed([]byte("\x1b[" + strings.Repeat("1;", maxSequenceBody)))
	if p.Pending() {
		t.Fatal("a runaway sequence is still buffered")
	}
	// And the parser still works afterwards.
	events := p.Feed([]byte("a"))
	if len(events) != 1 {
		t.Fatalf("got %+v, want the parser to have recovered", events)
	}
}

func TestMalformedSequenceRecoversAtTheOffendingByte(t *testing.T) {
	// A byte that cannot appear inside a sequence ends it. What follows is ordinary
	// input and must not be swallowed with the malformed prefix.
	events := feed("\x1b[1\x01a")
	if len(events) != 2 {
		t.Fatalf("got %+v, want the control chord and the letter", events)
	}
	if got := events[0].(Key); !got.IsRune('a', Ctrl) {
		t.Fatalf("first = %+v, want Ctrl+A", got)
	}
	if got := events[1].(Key); !got.IsRune('a', 0) {
		t.Fatalf("second = %+v, want the letter", got)
	}
}

func TestUnknownSequencesProduceNothing(t *testing.T) {
	for _, in := range []string{"\x1b[9999x", "\x1bOZ", "\x1b[99~"} {
		if events := feed(in); len(events) != 0 {
			t.Errorf("%q produced %+v, want nothing", in, events)
		}
	}
}

func TestKeyMatching(t *testing.T) {
	k := Key{Code: Character, Rune: 'c', Mods: Ctrl}
	if !k.IsRune('c', Ctrl) {
		t.Error("Ctrl+C does not match itself")
	}
	// Exactly, not at least: a binding on Ctrl+C that also fired for Ctrl+Shift+C
	// would swallow a keystroke it never claimed.
	withShift := Key{Code: Character, Rune: 'c', Mods: Ctrl | Shift}
	if withShift.IsRune('c', Ctrl) {
		t.Error("Ctrl+Shift+C matched a binding on Ctrl+C")
	}
	enter := Key{Code: Enter}
	if !enter.Is(Enter, 0) {
		t.Error("Enter does not match itself")
	}
	altEnter := Key{Code: Enter, Mods: Alt}
	if altEnter.Is(Enter, 0) {
		t.Error("Alt+Enter matched a binding on Enter")
	}
}

func TestKeyDownCoversRepeats(t *testing.T) {
	// Most handlers want this: holding a key stops working on terminals that report
	// repeats if only a press counts.
	repeat := Key{Transition: Repeat}
	if !repeat.Down() {
		t.Error("a repeat does not count as going down")
	}
	release := Key{Transition: Release}
	if release.Down() {
		t.Error("a release counts as going down")
	}
}

func TestKeyString(t *testing.T) {
	for _, tc := range []struct {
		key  Key
		want string
	}{
		{Key{Code: Character, Rune: 'c', Mods: Ctrl}, "ctrl+c"},
		{Key{Code: Character, Rune: ' '}, "space"},
		{Key{Code: Character, Rune: ' ', Mods: Ctrl}, "ctrl+space"},
		{Key{Code: Enter}, "enter"},
		{Key{Code: Backtab}, "shift+tab"},
		{Key{Code: F5}, "f5"},
		{Key{Code: Up, Mods: Ctrl | Shift}, "ctrl+shift+up"},
		{Key{Code: Character, Rune: 'x', Mods: Alt | Super}, "alt+super+x"},
	} {
		if got := tc.key.String(); got != tc.want {
			t.Errorf("%+v = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestAnIdleParserHoldsNothing(t *testing.T) {
	var p Parser
	p.Feed([]byte("abc"))
	if p.buf != nil {
		t.Fatal("the buffer is still allocated after everything decoded")
	}
}
