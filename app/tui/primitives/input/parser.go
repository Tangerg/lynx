package input

import (
	"image"
	"unicode/utf8"
)

const esc = 0x1b

const (
	// maxSequenceBody caps a control sequence's parameter section. A real key or
	// mouse report is an order of magnitude shorter; anything longer is garbage,
	// and buffering it would let a stream that never sends a final byte grow
	// memory without limit.
	maxSequenceBody = 64

	// maxPaste bounds what one paste may accumulate. A terminal that opens a
	// paste and never closes it would otherwise swallow everything typed
	// afterwards into a buffer that only grows. On reaching the bound the text so
	// far is delivered and paste mode ends, so nothing is lost — a consumer
	// inserting both halves gets what was pasted.
	maxPaste = 8 << 20
)

// pasteClose is the sequence a terminal sends to end a bracketed paste.
var pasteClose = []byte{esc, '[', '2', '0', '1', '~'}

// Parser decodes terminal bytes into events, incrementally.
//
// Bytes are handed to [Parser.Feed] exactly as they arrived, at whatever
// boundaries the read produced. Anything not yet unambiguous stays buffered:
// escape sequences and multi-byte characters routinely arrive in pieces, and a
// decoder that assumed otherwise would drop keys under load.
//
// One case cannot be resolved by waiting. A lone escape byte is either the Escape
// key or the start of a sequence whose remainder has not arrived, and only time
// tells the difference. [Parser.Pending] reports that something is waiting, and
// [Parser.Flush] declares the wait over.
//
// Not safe for concurrent use: it belongs to whichever goroutine reads the
// terminal.
type Parser struct {
	buf []byte
	// pasting is set between a paste's opening and closing sequences, when bytes
	// are text rather than input to interpret.
	pasting bool
	paste   []byte
}

// Feed adds bytes and returns everything now decodable.
func (p *Parser) Feed(b []byte) []Event {
	p.buf = append(p.buf, b...)
	return p.drain(false)
}

// Flush resolves what only time could resolve and returns the result.
//
// A buffered escape becomes the Escape key, and anything after it is re-read as
// ordinary input. A half-arrived character is dropped, since the rest is never
// coming. A paste in progress is left alone: it is incomplete rather than
// ambiguous, and cutting it short would corrupt the text.
func (p *Parser) Flush() []Event { return p.drain(true) }

// Pending reports whether bytes are waiting for more to make sense of them. It is
// what tells a loop to arm the timer that will call [Parser.Flush].
func (p *Parser) Pending() bool { return len(p.buf) > 0 }

// drain decodes as much as it can. When final, trailing ambiguity is resolved
// rather than kept.
func (p *Parser) drain(final bool) []Event {
	var events []Event
	for {
		if p.pasting {
			text, done := p.readPaste()
			if !done {
				return events
			}
			events = append(events, Paste{Text: text})
			continue
		}
		if len(p.buf) == 0 {
			return events
		}
		n, ev, done := p.decode(p.buf)
		if done {
			p.take(n)
			if ev != nil {
				events = append(events, ev)
			}
			continue
		}
		if !final {
			return events
		}
		if p.buf[0] == esc {
			// Nothing followed it in time, so it was the key.
			events = append(events, Key{Code: Esc})
			p.take(1)
			continue
		}
		// A character whose remaining bytes will never arrive. Dropping one byte
		// rather than the buffer keeps whatever follows decodable.
		p.take(1)
	}
}

// take drops n decoded bytes, releasing the buffer once it is empty so an idle
// parser holds nothing.
func (p *Parser) take(n int) {
	p.buf = p.buf[n:]
	if len(p.buf) == 0 {
		p.buf = nil
	}
}

// readPaste moves buffered bytes into the paste until its closing sequence is
// found, reporting whether the paste is complete.
func (p *Parser) readPaste() (string, bool) {
	i := 0
	for i < len(p.buf) {
		if p.buf[i] != esc {
			p.paste = append(p.paste, p.buf[i])
			i++
			if len(p.paste) >= maxPaste {
				p.take(i)
				return p.endPaste(), true
			}
			continue
		}
		rest := p.buf[i:]
		switch shared := commonPrefix(rest, pasteClose); {
		case shared == len(pasteClose):
			p.take(i + len(pasteClose))
			return p.endPaste(), true
		case shared == len(rest):
			// What is left could still become the closing sequence.
			p.buf = p.buf[i:]
			return "", false
		default:
			// It was not the closing sequence, so it is pasted text.
			p.paste = append(p.paste, esc)
			i++
		}
	}
	p.buf = nil
	return "", false
}

func (p *Parser) endPaste() string {
	text := string(p.paste)
	p.paste, p.pasting = nil, false
	return text
}

// decode reads one event from the front of b. It reports how many bytes it
// consumed, the event — nil when the bytes were understood but mean nothing to
// report — and whether it got far enough to decide. When it did not, no bytes
// were consumed.
func (p *Parser) decode(b []byte) (n int, ev Event, done bool) {
	switch c := b[0]; {
	case c == esc:
		return p.decodeEscape(b)
	case c == 0x0d:
		return 1, Key{Code: Enter}, true
	case c == 0x09:
		return 1, Key{Code: Tab}, true
	case c == 0x7f, c == 0x08:
		return 1, Key{Code: Backspace}, true
	case c >= 0x01 && c <= 0x1a:
		// Ctrl with a letter, which is what the terminal sends for it. Enter, Tab
		// and Backspace fall in this range too and are answered above, where their
		// own names are the truer report.
		return 1, Key{Code: Character, Rune: rune('a' + c - 1), Mods: Ctrl}, true
	case c < 0x20:
		// The rest of the C0 block. Each is Ctrl with a punctuation key, and
		// naming them is what makes those chords bindable at all.
		return 1, Key{Code: Character, Rune: c0Rune(c), Mods: Ctrl}, true
	default:
		if !utf8.FullRune(b) {
			return 0, nil, false
		}
		r, size := utf8.DecodeRune(b)
		if r == utf8.RuneError && size == 1 {
			return 1, nil, true // not valid UTF-8: dropped
		}
		return size, Key{Code: Character, Rune: r}, true
	}
}

// c0Rune names the C0 control bytes that are not keys of their own.
func c0Rune(c byte) rune {
	switch c {
	case 0x00:
		return ' ' // Ctrl+Space
	case 0x1c:
		return '\\'
	case 0x1d:
		return ']'
	case 0x1e:
		return '^'
	case 0x1f:
		return '_'
	default:
		return rune('a' + c - 1)
	}
}

// decodeEscape reads a sequence introduced by the escape byte.
func (p *Parser) decodeEscape(b []byte) (n int, ev Event, done bool) {
	if len(b) == 1 {
		return 0, nil, false // could be the key, could be a sequence: wait
	}
	switch second := b[1]; {
	case second == '[':
		return p.decodeControl(b)
	case second == 'O':
		if len(b) < 3 {
			return 0, nil, false
		}
		if code, ok := applicationKey(b[2]); ok {
			return 3, Key{Code: code}, true
		}
		return 3, nil, true
	case second == esc:
		// Two in a row: the first was the key, and the second starts again.
		return 1, Key{Code: Esc}, true
	case second == 0x0d:
		// Alt+Enter, which terminals send this way rather than as a modified-key
		// report.
		return 2, Key{Code: Enter, Mods: Alt}, true
	case second < 0x20, second == 0x7f:
		// A control byte cannot continue the sequence, so the escape stood alone
		// and the control byte is read on the next pass.
		return 1, Key{Code: Esc}, true
	default:
		if !utf8.FullRune(b[1:]) {
			return 0, nil, false
		}
		r, size := utf8.DecodeRune(b[1:])
		if r == utf8.RuneError && size == 1 {
			return 1, Key{Code: Esc}, true
		}
		return 1 + size, Key{Code: Character, Rune: r, Mods: Alt}, true
	}
}

// decodeControl reads a control sequence: a parameter section, then a final byte
// that says what the sequence was.
func (p *Parser) decodeControl(b []byte) (n int, ev Event, done bool) {
	i := 2
	for i < len(b) && b[i] >= 0x20 && b[i] <= 0x3f {
		i++
	}
	if i >= len(b) {
		if i-2 > maxSequenceBody {
			return len(b), nil, true // no final byte is coming: discard
		}
		return 0, nil, false
	}
	final := b[i]
	if final < 0x40 || final > 0x7e {
		// A byte that cannot appear in a control sequence. Drop the malformed
		// prefix and start again at the byte that proved it malformed.
		return i, nil, true
	}
	n = i + 1
	ps := parseParams(string(b[2:i]))

	switch {
	case ps.mouse() && (final == 'M' || final == 'm'):
		return n, decodeMouse(ps, final == 'M'), true
	case ps.empty() && final == 'I':
		return n, FocusIn{}, true
	case ps.empty() && final == 'O':
		return n, FocusOut{}, true
	}

	switch final {
	case 'u':
		return n, decodeExtendedKey(ps), true
	case '~':
		return n, p.decodeNumberedKey(ps), true
	case 'Z':
		mods, transition, ok := ps.keyMeta()
		if !ok {
			return n, nil, true
		}
		return n, Key{Code: Backtab, Mods: mods | Shift, Transition: transition}, true
	default:
		code, ok := cursorKey(final)
		if !ok {
			return n, nil, true // a sequence this program has no use for
		}
		mods, transition, ok := ps.keyMeta()
		if !ok {
			return n, nil, true
		}
		return n, Key{Code: code, Mods: mods, Transition: transition}, true
	}
}

// decodeNumberedKey reads the sequences that name a key by number, which is also
// how a terminal announces a paste.
func (p *Parser) decodeNumberedKey(ps params) Event {
	switch num := ps.first(); num {
	case pasteOpen:
		p.pasting = true
		return nil
	case pasteCloseNum:
		return nil // a closing sequence with no paste open
	default:
		code, ok := numberedKey(num)
		if !ok {
			return nil
		}
		mods, transition, ok := ps.keyMeta()
		if !ok {
			return nil
		}
		return Key{Code: code, Mods: mods, Transition: transition}
	}
}

const (
	pasteOpen     = 200
	pasteCloseNum = 201
)

// decodeExtendedKey reads the Kitty keyboard protocol's key report, which is the
// only form that distinguishes releases from presses and can say what text a key
// produced.
func decodeExtendedKey(ps params) Event {
	if ps.empty() || ps.count() > 3 {
		return nil // a bare sequence here is a cursor report, not a key
	}
	primary := ps.groups[0]
	if len(primary) == 0 || len(primary) > 3 || primary[0] <= 0 {
		return nil
	}
	// Alternate key codes are accepted and then ignored: reporting the key that
	// was pressed is this type's job, and reporting which key it would have been
	// under another layout is not.
	for _, alternate := range primary[1:] {
		if alternate < 0 || (alternate != 0 && !utf8.ValidRune(rune(alternate))) {
			return nil
		}
	}
	mods, transition, ok := ps.keyMeta()
	if !ok {
		return nil
	}
	text, ok := ps.text()
	if !ok {
		return nil
	}
	code, r, ok := extendedKeyCode(primary[0])
	if !ok {
		return nil
	}
	return Key{Code: code, Rune: r, Mods: mods, Transition: transition, Text: text}
}

// extendedKeyCode maps a Kitty key number onto a [Code].
//
// Numbers in the Unicode private-use area that this program does not recognise
// are rejected rather than passed through: emitting one as a character would put
// a glyph nobody typed into the text.
func extendedKeyCode(num int) (Code, rune, bool) {
	if code, ok := extendedKeys[num]; ok {
		return code, 0, true
	}
	if num >= 57364 && num <= 57375 {
		return F1 + Code(num-57364), 0, true
	}
	r := rune(num)
	if !utf8.ValidRune(r) || (num >= 0xe000 && num <= 0xf8ff) {
		return 0, 0, false
	}
	return Character, r, true
}

var extendedKeys = map[int]Code{
	8: Backspace, 127: Backspace, 57347: Backspace,
	9: Tab, 57346: Tab,
	13: Enter, 57345: Enter,
	27: Esc, 57344: Esc,
	57348: Insert,
	57349: Delete,
	57350: Left,
	57351: Right,
	57352: Up,
	57353: Down,
	57354: PageUp,
	57355: PageDown,
	57356: Home,
	57357: End,
}

// decodeMouse reads an SGR mouse report. down distinguishes the final byte that
// means "went down or moved" from the one that means "came up".
func decodeMouse(ps params, down bool) Event {
	if ps.count() < 3 {
		return nil
	}
	bits, x, y := ps.at(0), ps.at(1), ps.at(2)
	if bits < 0 || x < 0 || y < 0 {
		return nil // a malformed report says nothing about where the mouse is
	}
	// The terminal counts from one; everything above this package counts from zero.
	ev := Mouse{Pos: image.Pt(max(x-1, 0), max(y-1, 0)), Mods: mouseMods(bits)}
	switch {
	case bits&64 != 0:
		switch bits & 3 {
		case 0:
			ev.Action = WheelUp
		case 1:
			ev.Action = WheelDown
		default:
			return nil // horizontal wheel, which nothing here reads
		}
	case bits&32 != 0:
		ev.Button = mouseButton(bits & 3)
		if ev.Button == ButtonNone {
			ev.Action = MouseMove
		} else {
			ev.Action = MouseDrag
		}
	default:
		ev.Button = mouseButton(bits & 3)
		if down {
			ev.Action = MouseDown
		} else {
			ev.Action = MouseUp
		}
	}
	return ev
}

func mouseMods(bits int) Mods {
	var mods Mods
	if bits&4 != 0 {
		mods |= Shift
	}
	if bits&8 != 0 {
		mods |= Alt
	}
	if bits&16 != 0 {
		mods |= Ctrl
	}
	return mods
}

func mouseButton(bits int) Button {
	switch bits {
	case 0:
		return ButtonLeft
	case 1:
		return ButtonMiddle
	case 2:
		return ButtonRight
	default:
		return ButtonNone
	}
}

// commonPrefix is how many leading bytes a and b share.
func commonPrefix(a, b []byte) int {
	n := min(len(a), len(b))
	for i := range n {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func cursorKey(final byte) (Code, bool) {
	code, ok := cursorKeys[final]
	return code, ok
}

var cursorKeys = map[byte]Code{
	'A': Up, 'B': Down, 'C': Right, 'D': Left,
	'H': Home, 'F': End,
	'P': F1, 'Q': F2, 'R': F3, 'S': F4,
}

// applicationKey reads the keypad-mode form some terminals use for arrows and the
// first four function keys.
func applicationKey(c byte) (Code, bool) {
	code, ok := applicationKeys[c]
	return code, ok
}

var applicationKeys = map[byte]Code{
	'P': F1, 'Q': F2, 'R': F3, 'S': F4,
	'A': Up, 'B': Down, 'C': Right, 'D': Left,
	'H': Home, 'F': End,
}

func numberedKey(num int) (Code, bool) {
	code, ok := numberedKeys[num]
	return code, ok
}

var numberedKeys = map[int]Code{
	1: Home, 7: Home,
	2: Insert,
	3: Delete,
	4: End, 8: End,
	5:  PageUp,
	6:  PageDown,
	11: F1, 12: F2, 13: F3, 14: F4, 15: F5,
	17: F6, 18: F7, 19: F8, 20: F9, 21: F10,
	23: F11, 24: F12,
}
