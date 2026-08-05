package atoms

import (
	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/text"
)

// Braille is the spinner every terminal renders and nobody has to think about: it
// occupies one cell, it is the same width in every frame, and it reads as motion
// rather than as characters changing.
var Braille = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Dots is the fallback for terminals without Braille coverage.
var Dots = []string{"·", "•", "●", "•"}

// Spinner shows that something is happening.
//
// It holds a frame number rather than a clock. Whoever runs the loop decides when
// time passes, which is what lets a test step it deterministically and what keeps
// an idle UI from waking up to animate something nobody is waiting for.
type Spinner struct {
	// Frames are the glyphs cycled through. Empty means [Braille].
	Frames []string
	Style  grid.Style
	// Label follows the glyph, if there is room.
	Label      string
	LabelStyle grid.Style

	frame int
}

// Tick advances the animation by one frame.
func (s *Spinner) Tick() { s.frame++ }

// Height is one row.
func (s *Spinner) Height(int) int { return 1 }

// Draw writes the current frame and its label.
func (s *Spinner) Draw(v grid.View) {
	w, h := v.Size()
	if w <= 0 || h <= 0 {
		return
	}
	frames := s.Frames
	if len(frames) == 0 {
		frames = Braille
	}
	glyph := frames[((s.frame%len(frames))+len(frames))%len(frames)]
	x := v.Text(0, 0, glyph, s.Style)
	if s.Label == "" || x+1 >= w {
		return
	}
	v.Text(x+1, 0, text.Truncate(s.Label, w-x-1, "…"), s.LabelStyle)
}

// Scrollbar shows where a window sits in something taller than itself.
//
// It is drawn, not interacted with. Scrolling is the business of whatever owns the
// content and the keys; this only says where you are, which is the part a user
// cannot work out for themselves.
type Scrollbar struct {
	// Total is how many rows of content there are, Window how many are shown, and
	// Offset how many are above the window.
	Total, Window, Offset int
	// Track and Thumb draw the bar. Their zero values use a light track and a solid
	// thumb, which reads at a glance without drawing the eye.
	Track, Thumb           string
	TrackStyle, ThumbStyle grid.Style
}

// Needed reports whether there is anything to indicate. A bar drawn when
// everything already fits is a column of decoration.
func (s Scrollbar) Needed() bool { return s.Total > s.Window && s.Window > 0 }

// Draw paints the bar down the first column of v.
func (s Scrollbar) Draw(v grid.View) {
	_, h := v.Size()
	if h <= 0 {
		return
	}
	track, thumb := s.Track, s.Thumb
	if track == "" {
		track = "│"
	}
	if thumb == "" {
		thumb = "█"
	}
	if !s.Needed() {
		for y := range h {
			v.Text(0, y, track, s.TrackStyle)
		}
		return
	}

	// At least one row of thumb, however long the content: a thumb rounded down to
	// nothing tells the user nothing.
	size := max(h*s.Window/s.Total, 1)
	scrollable := s.Total - s.Window
	span := h - size
	top := 0
	if scrollable > 0 && span > 0 {
		top = min(s.Offset, scrollable) * span / scrollable
	}
	for y := range h {
		if y >= top && y < top+size {
			v.Text(0, y, thumb, s.ThumbStyle)
			continue
		}
		v.Text(0, y, track, s.TrackStyle)
	}
}
