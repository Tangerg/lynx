package parts

import (
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/tui/atoms"
	"github.com/Tangerg/lynx/app/tui/atoms/theme"
	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/input"
	"github.com/Tangerg/lynx/app/tui/primitives/text"
)

// Approval asks the user to allow or refuse something, and takes their answer.
//
// It takes the keyboard while it is open. A prompt that could be typed past would be
// a prompt the user answers by accident, and what it is asking about is a change to
// their files.
type Approval struct {
	Theme theme.Theme
	// Answer is called with the decision. It is the owner's job to send it on and to
	// close the prompt.
	Answer func(client.Decision)

	request client.Approval
	choices *atoms.List[choice]
	box     atoms.Box
}

// choice is one answer the prompt offers.
type choice struct {
	label    string
	decision client.Decision
	// danger marks the answer that lets something happen, so it does not read as the
	// safe one.
	danger bool
}

// NewApproval returns a closed prompt.
func NewApproval(t theme.Theme) *Approval {
	a := &Approval{
		Theme: t,
		box: atoms.Box{
			Border:  atoms.Rounded,
			Style:   t.Warning,
			Padding: atoms.Symmetric(0, 1),
		},
	}
	a.choices = &atoms.List[choice]{
		Keys: atoms.DefaultListKeys(),
		Wrap: true,
		Row: func(v grid.View, c choice, selected bool) {
			style := t.Text
			if c.danger {
				style = t.Warning
			}
			marker := "  "
			if selected {
				marker = "▸ "
				style = style.Merge(grid.Style{Attr: grid.Bold})
			}
			v.Text(0, 0, marker, t.Accent)
			v.Text(2, 0, c.label, style)
		},
	}
	return a
}

// Open shows the prompt for a request.
func (a *Approval) Open(request client.Approval) {
	a.request = request
	a.choices.SetItems([]choice{
		{label: "Allow once", decision: client.Decision{Approved: true}, danger: true},
		{label: "Allow and stop asking", decision: client.Decision{Approved: true, Remember: true}, danger: true},
		{label: "Refuse", decision: client.Decision{Reason: "declined by the user"}},
	})
	// The safe answer is the one selected: an answer given by reflex should be the one
	// that changes nothing.
	a.choices.Select(2)
}

// Close hides the prompt.
func (a *Approval) Close() { a.request = client.Approval{} }

// Open reports whether the prompt is showing.
func (a *Approval) Showing() bool { return a.request.InterruptID != "" }

// Request is what is being asked about.
func (a *Approval) Request() client.Approval { return a.request }

// Handle answers the prompt. While it is open it consumes everything, because
// nothing else on the screen can be acted on until this is settled.
func (a *Approval) Handle(ev input.Event) bool {
	if !a.Showing() {
		return false
	}
	if a.choices.Handle(ev) {
		return true
	}
	key, ok := ev.(input.Key)
	if !ok || !key.Down() {
		return true
	}
	switch {
	case key.Is(input.Enter, 0):
		a.answer()
	case key.Is(input.Esc, 0):
		// Escape refuses. It is the answer that changes nothing, which is what
		// Escape means everywhere else.
		a.send(client.Decision{Reason: "declined by the user"})
	case key.IsRune('y', 0):
		a.send(client.Decision{Approved: true})
	case key.IsRune('n', 0):
		a.send(client.Decision{Reason: "declined by the user"})
	}
	return true
}

func (a *Approval) answer() {
	if c, ok := a.choices.Current(); ok {
		a.send(c.decision)
	}
}

func (a *Approval) send(d client.Decision) {
	if a.Answer != nil {
		a.Answer(d)
	}
}

// Height is how tall the prompt needs to be at a width: the question, its detail,
// the diff it is about, and the answers.
func (a *Approval) Height(width int) int {
	if !a.Showing() {
		return 0
	}
	ow, oh := a.box.Overhead()
	room := max(width-ow, 1)
	rows := 1 // the question
	if a.request.Detail != "" {
		rows += len(text.WrapAll(linesOf(a.request.Detail, a.Theme.Muted), max(room-2, 1)))
	}
	if a.request.Diff != "" {
		rows += 1 + min(diffRows(a.request.Diff), maxApprovalDiffRows)
	}
	return rows + 1 + len(a.choices.Items) + oh
}

// maxApprovalDiffRows caps the preview. A prompt taller than the screen cannot be
// answered, and the change can be read in full in the transcript afterwards.
const maxApprovalDiffRows = 12

// Draw paints the prompt.
func (a *Approval) Draw(v grid.View) {
	if !a.Showing() {
		return
	}
	inner := a.box.Draw(v)
	width, height := inner.Size()
	if width <= 0 || height <= 0 {
		return
	}

	y := 0
	question := text.Line{
		{Text: "? ", Style: a.Theme.Warning},
		{Text: text.Truncate(a.request.Title, max(width-2, 1), "…"), Style: a.Theme.Strong},
	}
	question.Draw(inner, 0, y)
	y++

	if a.request.Detail != "" {
		for _, row := range text.WrapAll(linesOf(a.request.Detail, a.Theme.Muted), max(width-2, 1)) {
			if y >= height {
				return
			}
			row.Draw(inner, 2, y)
			y++
		}
	}
	if a.request.Diff != "" && y < height {
		y++
		y = a.drawDiff(inner, y, width, height)
	}
	if y < height {
		y++
	}
	if rest := height - y; rest > 0 {
		a.choices.Draw(inner.Sub(grid.Rect(0, y, width, rest)))
	}
}

func (a *Approval) drawDiff(v grid.View, y, width, height int) int {
	lines := strings.Split(strings.TrimRight(a.request.Diff, "\n"), "\n")
	shown := min(len(lines), maxApprovalDiffRows)
	for _, line := range lines[:shown] {
		if y >= height {
			return y
		}
		style := a.Theme.Context
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			style = a.Theme.Subtle
		case strings.HasPrefix(line, "@@"):
			style = a.Theme.Info
		case strings.HasPrefix(line, "+"):
			style = a.Theme.Added
		case strings.HasPrefix(line, "-"):
			style = a.Theme.Removed
		}
		// Drawn through a line rather than straight onto the view, so that a tab in a
		// diff becomes the columns it stands for. A diff of indented code with its
		// indentation dropped is not a diff anybody can read.
		text.Of(line, style).Truncate(max(width-2, 1), "…").Draw(v, 2, y)
		y++
	}
	if rest := len(lines) - shown; rest > 0 && y < height {
		v.Text(2, y, "… "+more(rest), a.Theme.Subtle)
		y++
	}
	return y
}

// Bindings are the prompt's keys, for the hint row.
func (a *Approval) Bindings() []atoms.Binding {
	return []atoms.Binding{
		{Key: input.Key{Code: input.Up}, Does: "choose"},
		{Key: input.Key{Code: input.Enter}, Does: "answer"},
		{Key: input.Key{Code: input.Character, Rune: 'y'}, Does: "allow"},
		{Key: input.Key{Code: input.Character, Rune: 'n'}, Does: "refuse"},
	}
}

func diffRows(diff string) int {
	return strings.Count(strings.TrimRight(diff, "\n"), "\n") + 1
}
