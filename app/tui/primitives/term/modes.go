package term

import "strings"

// Terminal modes, each written as a pair: what turns it on, and what puts it back.
//
// Every one of these outlives the process if it is not put back. A program that
// exits with mouse reporting on leaves the shell printing escape sequences when
// the user moves the mouse; one that exits on the alternate screen loses whatever
// the user had on screen before it started. Restoring them is not tidiness.
const (
	altScreenOn  = "\x1b[?1049h"
	altScreenOff = "\x1b[?1049l"

	// Mouse reporting: any-event tracking so hover is reported and not only drags,
	// plus the extended coordinate encoding, which is the only one that works past
	// column 223.
	mouseOn  = "\x1b[?1003h\x1b[?1006h"
	mouseOff = "\x1b[?1006l\x1b[?1003l"

	focusOn  = "\x1b[?1004h"
	focusOff = "\x1b[?1004l"

	// The Kitty keyboard protocol's progressive enhancement. The flags ask for
	// unambiguous key codes, key release and repeat events, alternate key codes and
	// the text a key produced. A terminal that does not implement it ignores the
	// request, and the disable form pops whatever was pushed.
	keyboardOn  = "\x1b[>31u"
	keyboardOff = "\x1b[<u"

	pasteOn  = "\x1b[?2004h"
	pasteOff = "\x1b[?2004l"

	cursorShow = "\x1b[?25h"
)

// modes is the set of terminal modes a session turns on.
type modes struct {
	altScreen bool
	mouse     bool
	focus     bool
	keyboard  bool
}

// mode pairs one mode's enable and disable sequences with whether it is wanted.
type mode struct {
	on, off string
	wanted  bool
}

// sequence lists the modes in the order they are turned on. Bracketed paste is
// always wanted: a terminal that cannot tell a paste from typing turns pasted code
// into keystrokes, and there is no reason to want that.
func (m modes) sequence() []mode {
	return []mode{
		{altScreenOn, altScreenOff, m.altScreen},
		{mouseOn, mouseOff, m.mouse},
		{focusOn, focusOff, m.focus},
		{keyboardOn, keyboardOff, m.keyboard},
		{pasteOn, pasteOff, true},
	}
}

// enter is what to write to take the terminal over.
func (m modes) enter() string {
	var b strings.Builder
	for _, mode := range m.sequence() {
		if mode.wanted {
			b.WriteString(mode.on)
		}
	}
	return b.String()
}

// leave is what to write to give the terminal back: every mode that was turned on,
// turned off in the opposite order, and then the cursor shown, because a frame may
// have hidden it.
func (m modes) leave() string {
	seq := m.sequence()
	var b strings.Builder
	for i := len(seq) - 1; i >= 0; i-- {
		if seq[i].wanted {
			b.WriteString(seq[i].off)
		}
	}
	b.WriteString(cursorShow)
	return b.String()
}
