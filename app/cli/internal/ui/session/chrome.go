package session

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

type workflowView struct {
	theme  kit.Theme
	glyphs kit.Glyphs
	items  []client.PlanItem
}

func newWorkflowView(theme kit.Theme, glyphs kit.Glyphs) workflowView {
	return workflowView{theme: theme, glyphs: glyphs}
}

func (w *workflowView) Set(items []client.PlanItem) {
	w.items = append(w.items[:0], items...)
}

func (w *workflowView) Reset() { clear(w.items); w.items = w.items[:0] }

func (w *workflowView) Measure(int) int {
	if len(w.items) == 0 {
		return 0
	}
	return len(w.items) + 2
}

func (w *workflowView) Draw(view grid.View) {
	if len(w.items) == 0 {
		return
	}
	box := kit.Box{Theme: w.theme, Glyphs: w.glyphs, Title: "run plan", Padding: layout.Symmetric(0, 1)}
	inner := box.Draw(view)
	width, height := inner.Size()
	for row, item := range w.items {
		if row >= height {
			return
		}
		mark, label, style := w.glyphs.Free, "pending", w.theme.Subtle
		switch item.Status {
		case client.PlanActive:
			mark, label, style = w.glyphs.Marker, "active", w.theme.Accent
		case client.PlanDone:
			mark, label, style = w.glyphs.Taken, "done", w.theme.Success
		}
		inner.Text(0, row, text.Truncate(mark+" "+item.Title, max(width-len(label)-2, 1), w.glyphs.Ellipsis), style)
		if len(label)+1 < width {
			inner.Text(width-len(label), row, label, style)
		}
	}
}

type statusView struct {
	theme   kit.Theme
	glyphs  kit.Glyphs
	doing   string
	elapsed string
	usage   client.Usage
	outcome client.Outcome
	status  kit.Status
	busy    bool
}

func (s *statusView) Measure(int) int { return 1 }

func (s *statusView) Draw(view grid.View) {
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	if s.busy {
		s.status.Theme, s.status.Glyphs = s.theme, s.glyphs
		s.status.Doing, s.status.Elapsed = s.doing, s.elapsed
		s.status.Draw(view)
		return
	}
	right := usageLabel(s.usage)
	style := s.theme.Muted
	switch s.outcome.Status {
	case client.OutcomeCompleted:
		style = s.theme.Success
	case client.OutcomeCanceled:
		style = s.theme.Warning
	case client.OutcomeFailed:
		style = s.theme.Danger
	}
	left := s.doing
	if right == "" || text.Width(right)+2 >= width {
		kit.Label{Text: left, Style: style, Ellipsis: s.glyphs.Ellipsis}.Draw(view)
		return
	}
	rightWidth := text.Width(right)
	kit.Label{Text: left, Style: style, Ellipsis: s.glyphs.Ellipsis}.Draw(view.Sub(grid.Rect(0, 0, width-rightWidth-1, 1)))
	view.Text(width-rightWidth, 0, right, s.theme.Subtle)
}

func (s *statusView) tick(elapsed time.Duration) {
	s.status.Tick()
	s.elapsed = fmt.Sprintf("%4.1fs", elapsed.Seconds())
}

func (s *statusView) settled(outcome client.Outcome, usage client.Usage) {
	s.outcome, s.usage, s.elapsed = outcome, usage, ""
	s.busy = false
	switch outcome.Status {
	case client.OutcomeCompleted:
		s.doing = "complete"
	case client.OutcomeCanceled:
		s.doing = "cancelled"
	case client.OutcomeFailed:
		s.doing = "failed: " + outcome.Error
	default:
		s.doing = "ready"
	}
}

func (s *statusView) active(label string) {
	s.doing = label
	s.outcome = client.Outcome{}
	s.elapsed = ""
	s.busy = true
}

func (s *statusView) note(label string) {
	s.doing = label
	s.outcome = client.Outcome{}
	s.busy = false
}

func usageLabel(usage client.Usage) string {
	if usage == (client.Usage{}) {
		return ""
	}
	parts := []string{"↑" + thousands(usage.InputTokens), "↓" + thousands(usage.OutputTokens)}
	if usage.CachedTokens > 0 {
		parts = append(parts, "cached "+thousands(usage.CachedTokens))
	}
	if usage.CostUSD > 0 {
		parts = append(parts, "$"+strconv.FormatFloat(usage.CostUSD, 'f', 4, 64))
	}
	if usage.Duration > 0 {
		parts = append(parts, compactTime(usage.Duration))
	}
	return strings.Join(parts, "  ")
}

func thousands(value int64) string {
	sign := ""
	if value < 0 {
		sign, value = "-", -value
	}
	digits := strconv.FormatInt(value, 10)
	for i := len(digits) - 3; i > 0; i -= 3 {
		digits = digits[:i] + "," + digits[i:]
	}
	return sign + digits
}

func compactTime(duration time.Duration) string {
	if duration < time.Second {
		return strconv.FormatInt(duration.Milliseconds(), 10) + "ms"
	}
	return strconv.FormatFloat(duration.Seconds(), 'f', 1, 64) + "s"
}
