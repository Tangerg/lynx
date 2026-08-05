package parts

import (
	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/tui/atoms"
	"github.com/Tangerg/lynx/app/tui/atoms/theme"
	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/text"
)

// Plan shows what the run means to do and how far it has got.
//
// It draws nothing when there is no plan. A pane holding an empty heading is worse
// than no pane: it takes room from the conversation and tells the reader that
// something is missing.
type Plan struct {
	Theme theme.Theme
	Items []client.PlanItem
	// Collapsed shows only the step in progress, for when the conversation needs the
	// room more than the plan does.
	Collapsed bool

	box atoms.Box
}

// NewPlan returns an empty plan pane.
func NewPlan(t theme.Theme) *Plan {
	return &Plan{
		Theme: t,
		box: atoms.Box{
			Border:     atoms.Rounded,
			Style:      t.Border,
			Padding:    atoms.Symmetric(0, 1),
			Title:      "Plan",
			TitleStyle: t.Heading,
		},
	}
}

// Height is how tall the pane needs to be, or zero when there is no plan.
func (p *Plan) Height(int) int {
	if len(p.Items) == 0 {
		return 0
	}
	_, oh := p.box.Overhead()
	return len(p.shown()) + oh
}

// Draw paints the plan.
func (p *Plan) Draw(v grid.View) {
	if len(p.Items) == 0 {
		return
	}
	inner := p.box.Draw(v)
	width, height := inner.Size()
	if width <= 0 || height <= 0 {
		return
	}
	for y, item := range p.shown() {
		if y >= height {
			return
		}
		mark, style := p.mark(item.Status)
		inner.Text(0, y, mark+" ", style)
		title := text.Truncate(item.Title, max(width-2, 1), "…")
		inner.Text(2, y, title, p.titleStyle(item.Status))
	}
}

// shown is the items to draw: all of them, or just the one in progress.
func (p *Plan) shown() []client.PlanItem {
	if !p.Collapsed {
		return p.Items
	}
	for _, item := range p.Items {
		if item.Status == client.PlanActive {
			return []client.PlanItem{item}
		}
	}
	// Nothing is in progress, so the first thing that is not done is what is next.
	for _, item := range p.Items {
		if item.Status != client.PlanDone {
			return []client.PlanItem{item}
		}
	}
	return nil
}

func (p *Plan) mark(status client.PlanStatus) (string, grid.Style) {
	switch status {
	case client.PlanDone:
		return "☑", p.Theme.Success
	case client.PlanActive:
		return "▸", p.Theme.Accent
	default:
		return "☐", p.Theme.Subtle
	}
}

// titleStyle dims what is done and leaves what is next plain, so the eye lands on
// the step in progress without anything having to blink.
func (p *Plan) titleStyle(status client.PlanStatus) grid.Style {
	switch status {
	case client.PlanDone:
		return p.Theme.Subtle
	case client.PlanActive:
		return p.Theme.Strong
	default:
		return p.Theme.Text
	}
}
