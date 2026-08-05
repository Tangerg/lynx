package parts

import (
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

// Composer is where the user writes.
//
// It owns the decision plain Enter makes, which the editor deliberately leaves open:
// here Enter sends and Alt+Enter breaks the line. That is the right way round for a
// chat — most messages are one line, and the common case should be the short
// keystroke.
type Composer struct {
	Theme kit.Theme
	// Marker introduces the field, so the eye finds where to type.
	Marker string
	// Submit is called with the text when the user sends it. It reports whether the
	// text was taken: a composer that cleared itself while the session was busy would
	// lose what the user wrote.
	Submit func(string) bool

	editor *headless.Editor
	box    kit.Box
}

// NewComposer returns a composer with the field a chat expects.
func NewComposer(t kit.Theme) *Composer {
	editor := headless.NewEditor()
	editor.Placeholder = "Ask anything, or /help"
	editor.Style = t.Text
	editor.PlaceholderStyle = t.Subtle
	editor.MaxRows = 8
	return &Composer{
		Theme:  t,
		Marker: "› ",
		editor: editor,
		box: kit.Box{
			Border:  kit.Rounded,
			Style:   t.Border,
			Padding: layout.Symmetric(0, 1),
		},
	}
}

// Editor is the field itself, for anything that needs to read or set the text.
func (c *Composer) Editor() *headless.Editor { return c.editor }

// Text is what has been written.
func (c *Composer) Text() string { return c.editor.Text() }

// Measure is how tall the composer needs to be at a width.
func (c *Composer) Measure(width int) int {
	overhead := c.box.Overhead()
	return c.editor.Measure(max(width-overhead.W-text.Width(c.Marker), 1)) + overhead.H
}

// Handle answers input, reporting whether it consumed the event.
func (c *Composer) Handle(ev input.Event) bool {
	if key, ok := ev.(input.Key); ok && key.Down() && key.Is(input.Enter, 0) {
		c.send()
		return true
	}
	return c.editor.Handle(ev)
}

// send hands the text over and clears the field, but only if it was taken.
func (c *Composer) send() {
	body := strings.TrimSpace(c.editor.Text())
	if body == "" || c.Submit == nil {
		return
	}
	if c.Submit(body) {
		c.editor.Clear()
	}
}

// Draw paints the field.
func (c *Composer) Draw(v grid.View) {
	inner := c.box.Draw(v)
	width, height := inner.Size()
	if width <= 0 || height <= 0 {
		return
	}
	marker := text.Width(c.Marker)
	inner.Text(0, 0, c.Marker, c.Theme.Accent)
	c.editor.Draw(inner.Sub(grid.Rect(marker, 0, max(width-marker, 0), height)))
}

// Status is the line under the composer: what the session is doing, and what it has
// cost.
//
// It is one row on purpose. Everything on it is reference material — a model name, a
// token count — and reference material that grew to two rows would be taking space
// from the conversation.
type Status struct {
	Theme kit.Theme
	// Left is what the session is doing.
	Left string
	// Model names the model in use.
	Model string
	// Usage is what the session has spent.
	Usage client.Usage
	// Spinner turns while something is happening. Nil means nothing is.
	Spinner *kit.Spinner
}

// Draw paints the status line: state on the left, cost on the right.
func (s Status) Draw(v grid.View) {
	width, height := v.Size()
	if width <= 0 || height <= 0 {
		return
	}
	right := s.cost()
	rightWidth := right.Width()

	x := 0
	if s.Spinner != nil {
		s.Spinner.Draw(v.Sub(grid.Rect(0, 0, min(2, width), 1)))
		x = 2
	}
	if s.Left != "" {
		room := max(width-x-rightWidth-2, 0)
		x += v.Text(x, 0, text.Truncate(s.Left, room, "…"), s.Theme.Muted)
	}
	if s.Model != "" {
		room := max(width-x-rightWidth-3, 0)
		if room > 0 {
			x += v.Text(x, 0, "  ", s.Theme.Muted)
			x += v.Text(x, 0, text.Truncate(s.Model, room, "…"), s.Theme.Subtle)
		}
	}
	if rightWidth > 0 && rightWidth <= width-x {
		right.Draw(v, width-rightWidth, 0)
	}
}

// cost is the usage half of the status line.
func (s Status) cost() text.Line {
	u := s.Usage
	if u.InputTokens == 0 && u.OutputTokens == 0 {
		return nil
	}
	line := text.Line{
		{Text: "↑ " + thousands(u.InputTokens), Style: s.Theme.Subtle},
		{Text: "  ↓ " + thousands(u.OutputTokens), Style: s.Theme.Subtle},
	}
	if u.CostUSD > 0 {
		line = append(line, text.Span{
			Text:  "  $" + strconv.FormatFloat(u.CostUSD, 'f', 4, 64),
			Style: s.Theme.Muted,
		})
	}
	return line
}

// duration prints a span the way a person reads one.
func duration(d time.Duration) string {
	if d < time.Second {
		return strconv.FormatInt(d.Milliseconds(), 10) + "ms"
	}
	return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + "s"
}

// thousands groups an integer for reading.
func thousands(n int64) string {
	if n < 0 {
		return "-" + thousands(-n)
	}
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}
