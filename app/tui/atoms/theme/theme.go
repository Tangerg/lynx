// Package theme names the colours the interface is built from.
//
// The names are roles, not colours: a widget asks for the style of a border or of
// muted text, never for a particular grey. That is what lets the whole interface
// change palette in one place, and what stops the same grey from being chosen
// twice with two slightly different values.
//
// Nothing here draws. A theme is a value that widgets are given, not a global they
// reach for: a global palette cannot be varied per pane, and a test cannot pin one.
package theme

import "github.com/Tangerg/lynx/app/tui/primitives/grid"

// Theme is the palette an interface is drawn with.
//
// The fields are styles rather than colours because a role often carries more than
// a colour: muted text is dim as well as grey, and a heading is bold. A widget that
// had to remember to add the dimming would be a widget that sometimes forgot.
type Theme struct {
	// Text is body text, and Muted is text present for reference rather than for
	// reading — timestamps, paths, counts.
	Text  grid.Style
	Muted grid.Style
	// Subtle is quieter than muted: structure the eye should skip unless it is
	// looking for it.
	Subtle grid.Style
	// Strong is emphasis within body text.
	Strong grid.Style
	// Heading titles a pane or a section.
	Heading grid.Style

	// Accent marks the thing the interface is about: the active item, the key in a
	// hint, the prompt marker.
	Accent grid.Style
	// Success, Warning, Danger and Info are outcomes. They are the only colours in
	// the interface that carry meaning on their own, which is why there are exactly
	// four of them.
	Success grid.Style
	Warning grid.Style
	Danger  grid.Style
	Info    grid.Style

	// Border draws a frame, and Divider a line between things inside one.
	Border  grid.Style
	Divider grid.Style
	// Selection is the row under the cursor.
	Selection grid.Style
	// Surface is a pane's background, and Sunken a well inside one — a tool's output,
	// a code block.
	Surface grid.Style
	Sunken  grid.Style

	// Added and Removed are the two halves of a diff. Nothing else in the interface
	// uses green and red on a background, so a diff is recognisable at a glance.
	Added   grid.Style
	Removed grid.Style
	// Context is a diff line that did not change.
	Context grid.Style
}

// Dark is the default theme: a cool slate, the same family the desktop interface
// uses, so the two do not look like different products.
//
// The greys are cool on purpose. A neutral grey beside the blue accent reads as
// slightly yellow, and the whole interface looks dusty.
func Dark() Theme {
	var (
		text    = grid.RGBColor(0xE2, 0xE6, 0xEF)
		muted   = grid.RGBColor(0x94, 0x9C, 0xB0)
		subtle  = grid.RGBColor(0x64, 0x6C, 0x80)
		accent  = grid.RGBColor(0x7A, 0xA2, 0xF7)
		green   = grid.RGBColor(0x7A, 0xC8, 0x8E)
		amber   = grid.RGBColor(0xD7, 0xA6, 0x5C)
		red     = grid.RGBColor(0xE8, 0x7D, 0x7D)
		cyan    = grid.RGBColor(0x6C, 0xB6, 0xC4)
		line    = grid.RGBColor(0x3A, 0x41, 0x52)
		surface = grid.RGBColor(0x16, 0x19, 0x22)
		sunken  = grid.RGBColor(0x1D, 0x21, 0x2C)
		select_ = grid.RGBColor(0x25, 0x2B, 0x3A)
		addedBG = grid.RGBColor(0x18, 0x2C, 0x21)
		goneBG  = grid.RGBColor(0x2E, 0x1C, 0x1F)
	)
	return Theme{
		Text:      grid.Style{FG: text},
		Muted:     grid.Style{FG: muted},
		Subtle:    grid.Style{FG: subtle},
		Strong:    grid.Style{FG: text, Attr: grid.Bold},
		Heading:   grid.Style{FG: text, Attr: grid.Bold},
		Accent:    grid.Style{FG: accent},
		Success:   grid.Style{FG: green},
		Warning:   grid.Style{FG: amber},
		Danger:    grid.Style{FG: red},
		Info:      grid.Style{FG: cyan},
		Border:    grid.Style{FG: line},
		Divider:   grid.Style{FG: line},
		Selection: grid.Style{BG: select_},
		Surface:   grid.Style{BG: surface},
		Sunken:    grid.Style{BG: sunken},
		Added:     grid.Style{FG: green, BG: addedBG},
		Removed:   grid.Style{FG: red, BG: goneBG},
		Context:   grid.Style{FG: muted},
	}
}

// Light is the same palette turned over, for a terminal on a light background.
func Light() Theme {
	var (
		text    = grid.RGBColor(0x1C, 0x21, 0x2C)
		muted   = grid.RGBColor(0x5C, 0x65, 0x78)
		subtle  = grid.RGBColor(0x8B, 0x93, 0xA5)
		accent  = grid.RGBColor(0x2E, 0x5C, 0xC8)
		green   = grid.RGBColor(0x1F, 0x7A, 0x45)
		amber   = grid.RGBColor(0x92, 0x5F, 0x0E)
		red     = grid.RGBColor(0xB4, 0x2D, 0x2D)
		cyan    = grid.RGBColor(0x1B, 0x6A, 0x78)
		line    = grid.RGBColor(0xD2, 0xD7, 0xE0)
		surface = grid.RGBColor(0xFA, 0xFB, 0xFD)
		sunken  = grid.RGBColor(0xF0, 0xF2, 0xF6)
		select_ = grid.RGBColor(0xE4, 0xE8, 0xF0)
		addedBG = grid.RGBColor(0xE7, 0xF6, 0xEC)
		goneBG  = grid.RGBColor(0xFB, 0xEA, 0xEA)
	)
	return Theme{
		Text:      grid.Style{FG: text},
		Muted:     grid.Style{FG: muted},
		Subtle:    grid.Style{FG: subtle},
		Strong:    grid.Style{FG: text, Attr: grid.Bold},
		Heading:   grid.Style{FG: text, Attr: grid.Bold},
		Accent:    grid.Style{FG: accent},
		Success:   grid.Style{FG: green},
		Warning:   grid.Style{FG: amber},
		Danger:    grid.Style{FG: red},
		Info:      grid.Style{FG: cyan},
		Border:    grid.Style{FG: line},
		Divider:   grid.Style{FG: line},
		Selection: grid.Style{BG: select_},
		Surface:   grid.Style{BG: surface},
		Sunken:    grid.Style{BG: sunken},
		Added:     grid.Style{FG: green, BG: addedBG},
		Removed:   grid.Style{FG: red, BG: goneBG},
		Context:   grid.Style{FG: muted},
	}
}
