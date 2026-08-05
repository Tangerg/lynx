package atoms

import (
	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/input"
)

// Scroll shows a window onto something taller than the space available.
//
// It holds two things: how many rows are hidden above the window, and whether the
// window is following the end of the content.
//
// Both are needed, and neither on its own will do. Holding only the offset means a
// live log stops showing what arrives. Holding only a distance from the end means a
// reader who scrolled up gets dragged forward every time something is appended:
// twenty rows from the end becomes thirty rows from the end, and the text under
// their eyes moves even though they did not ask it to.
//
// The zero value shows the start and does not follow, which is what a list of items
// wants. A transcript asks to follow, once, with [Scroll.ToBottom].
type Scroll struct {
	// offset is how many rows are hidden above the window.
	offset int
	// following makes the window stick to the end as content arrives.
	following bool
	// total and window are what the last layout measured, remembered so a scroll
	// arriving between frames is clamped against something real.
	total, window int
}

// Layout tells the scroll how much content there is and how much of it is shown.
//
// It runs once per frame, before anything is drawn. A following window moves to the
// new end; a window that is not following keeps its place, clamped against content
// that may have grown or shrunk since the last frame.
func (s *Scroll) Layout(total, window int) {
	s.total, s.window = max(total, 0), max(window, 0)
	if s.following {
		s.offset = s.max()
		return
	}
	s.clamp()
}

// Offset is how many rows are hidden above the window, which is what a scrollbar and
// a hit test both want.
func (s *Scroll) Offset() int { return s.offset }

// AtBottom reports whether the window is following the end of the content.
func (s *Scroll) AtBottom() bool { return s.following }

// By scrolls a number of rows: negative towards the start, positive towards the end.
//
// Reaching the end starts following again, which is what every log viewer does and
// what a reader means by scrolling to the bottom.
func (s *Scroll) By(rows int) {
	s.offset += rows
	s.clamp()
	s.following = s.offset >= s.max()
}

// ToBottom follows the end of the content.
func (s *Scroll) ToBottom() {
	s.following = true
	s.offset = s.max()
}

// ToTop shows the start of the content and stops following.
func (s *Scroll) ToTop() {
	s.following = false
	s.offset = 0
}

// Pages scrolls whole windows, keeping one row of overlap so the reader has
// something to recognise on the other side of the jump.
func (s *Scroll) Pages(n int) {
	s.By(n * max(s.window-1, 1))
}

// max is the largest offset that still shows a full window.
func (s *Scroll) max() int { return max(s.total-s.window, 0) }

func (s *Scroll) clamp() { s.offset = min(max(s.offset, 0), s.max()) }

// ScrollKeys are the keystrokes [Scroll.Handle] answers.
//
// They are a field rather than a constant so a container can re-bind them: the same
// widget appears in places where Escape means "back" and places where it means
// "close", and a widget that owned its keys would have to be told about both.
type ScrollKeys struct {
	Up, Down       Binding
	PageUp, PageDn Binding
	Top, Bottom    Binding
}

// DefaultScrollKeys are the bindings a terminal reader expects.
func DefaultScrollKeys() ScrollKeys {
	return ScrollKeys{
		Up:     Binding{Key: input.Key{Code: input.Up}, Does: "up"},
		Down:   Binding{Key: input.Key{Code: input.Down}, Does: "down"},
		PageUp: Binding{Key: input.Key{Code: input.PageUp}, Does: "page up"},
		PageDn: Binding{Key: input.Key{Code: input.PageDown}, Does: "page down"},
		Top:    Binding{Key: input.Key{Code: input.Home}, Does: "top"},
		Bottom: Binding{Key: input.Key{Code: input.End}, Does: "bottom"},
	}
}

// Handle scrolls in response to keys and the mouse wheel, reporting whether it
// consumed the event.
func (s *Scroll) Handle(ev input.Event, keys ScrollKeys) bool {
	if mouse, ok := ev.(input.Mouse); ok {
		switch mouse.Action {
		case input.WheelUp:
			s.By(-wheelRows)
			return true
		case input.WheelDown:
			s.By(wheelRows)
			return true
		default:
			return false
		}
	}
	switch {
	case keys.Up.Matches(ev):
		s.By(-1)
	case keys.Down.Matches(ev):
		s.By(1)
	case keys.PageUp.Matches(ev):
		s.Pages(-1)
	case keys.PageDn.Matches(ev):
		s.Pages(1)
	case keys.Top.Matches(ev):
		s.ToTop()
	case keys.Bottom.Matches(ev):
		s.ToBottom()
	default:
		return false
	}
	return true
}

// wheelRows is how far one notch of the wheel scrolls. Three is what a terminal
// sends for a line-based scroll and what every other program moves.
const wheelRows = 3

// Rows draws the visible slice of a set of rows.
//
// Each row is drawn by the caller's function, which is given a view one row tall.
// The rows are drawn rather than returned so a row can be as complicated as it
// likes without this type having to know.
func (s *Scroll) Rows(v grid.View, total int, row func(v grid.View, index int)) {
	width, height := v.Size()
	s.Layout(total, height)
	first := s.Offset()
	for y := range height {
		index := first + y
		if index >= total {
			return
		}
		row(v.Sub(grid.Rect(0, y, width, 1)), index)
	}
}
