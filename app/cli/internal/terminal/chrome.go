package terminal

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

const (
	headerMinWidth   = 32
	activityMinWidth = 28
	activityMaxRows  = 6
)

type sessionHeader struct {
	theme   kit.Theme
	glyphs  kit.Glyphs
	session agent.Session
	usage   agent.Usage
}

func newSessionHeader(theme kit.Theme, glyphs kit.Glyphs, session agent.Session) *sessionHeader {
	return &sessionHeader{theme: theme, glyphs: glyphs, session: session}
}

func (h *sessionHeader) SetSession(session agent.Session) { h.session = session }

func (h *sessionHeader) SetUsage(usage agent.Usage) { h.usage = usage }

func (h *sessionHeader) Measure(width int) int {
	if width < headerMinWidth {
		return 0
	}
	return 2
}

func (h *sessionHeader) Draw(view grid.View) {
	width, height := view.Size()
	if width < headerMinWidth || height <= 0 {
		return
	}
	right := headerUsageLabel(h.usage)
	rightWidth := text.Width(right)
	if rightWidth > 0 && rightWidth < width {
		view.Text(width-rightWidth, 0, right, h.theme.Subtle)
	} else {
		rightWidth = 0
	}

	available := width
	if rightWidth > 0 {
		available -= rightWidth + 2
	}
	if available <= 0 {
		return
	}
	workspace := displayWorkspace(h.session.Workspace)
	title := displayTitle(h.session)
	separator := "  " + h.glyphs.Bullet + "  "
	workspace = text.Truncate(workspace, available, h.glyphs.Ellipsis)
	x := view.Text(0, 0, workspace, h.theme.Context)
	remaining := available - x
	if remaining > text.Width(separator) {
		view.Text(x, 0, separator+text.Truncate(title, remaining-text.Width(separator), h.glyphs.Ellipsis), h.theme.Muted)
	}
}

func displayWorkspace(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == "" {
		return "workspace"
	}
	return path
}

func headerUsageLabel(usage agent.Usage) string {
	if usage.InputTokens == 0 && usage.OutputTokens == 0 {
		return ""
	}
	return "↑" + compactTokens(usage.InputTokens) + "  ↓" + compactTokens(usage.OutputTokens)
}

func compactTokens(tokens int64) string {
	abs := uint64(tokens)
	if tokens < 0 {
		abs = uint64(-(tokens + 1)) + 1
	}
	switch {
	case abs < 1_000:
		return strconv.FormatInt(tokens, 10)
	case abs < 10_000:
		return strconv.FormatFloat(float64(tokens)/1_000, 'f', 1, 64) + "k"
	case abs < 1_000_000:
		return strconv.FormatInt(tokens/1_000, 10) + "k"
	default:
		return strconv.FormatFloat(float64(tokens)/1_000_000, 'f', 1, 64) + "m"
	}
}

type activityView struct {
	theme  kit.Theme
	glyphs kit.Glyphs
	items  []agent.PlanItem
}

func newActivityView(theme kit.Theme, glyphs kit.Glyphs) *activityView {
	return &activityView{theme: theme, glyphs: glyphs}
}

func (a *activityView) Set(items []agent.PlanItem) {
	a.items = append(a.items[:0], items...)
}

func (a *activityView) Reset() {
	clear(a.items)
	a.items = a.items[:0]
}

func (a *activityView) Measure(width int) int {
	if len(a.items) == 0 || width < activityMinWidth {
		return 0
	}
	maximum := activityMaxRows
	if width < 60 {
		maximum = 4
	}
	return min(len(a.items)+1, maximum)
}

func (a *activityView) Draw(view grid.View) {
	width, height := view.Size()
	if len(a.items) == 0 || width < activityMinWidth || height <= 0 {
		return
	}
	done, active := activityProgress(a.items)
	header := a.glyphs.Expanded + " Plan"
	view.Text(0, 0, header, a.theme.Heading)
	progress := fmt.Sprintf("%d/%d", done, len(a.items))
	if active >= 0 {
		progress = fmt.Sprintf("step %d/%d", active+1, len(a.items))
	}
	if progressWidth := text.Width(progress); progressWidth+text.Width(header)+2 <= width {
		view.Text(width-progressWidth, 0, progress, a.theme.Subtle)
	}

	capacity := height - 1
	start, end := activityWindow(len(a.items), capacity, active)
	for row, index := 1, start; row < height && index < end; row, index = row+1, index+1 {
		item := a.items[index]
		mark, label, style := a.glyphs.Free, "pending", a.theme.Muted
		switch item.Status {
		case agent.PlanActive:
			mark, label, style = a.glyphs.Marker, "active", a.theme.Accent
		case agent.PlanDone:
			mark, label, style = a.glyphs.Taken, "done", a.theme.Success
		case agent.PlanPending:
		default:
		}
		view.Text(0, row, a.glyphs.Vertical, a.theme.Divider)
		labelWidth := text.Width(label)
		contentWidth := max(width-labelWidth-4, 1)
		view.Text(2, row, mark+" "+text.Truncate(item.Title, contentWidth, a.glyphs.Ellipsis), style)
		if labelWidth+4 < width {
			view.Text(width-labelWidth, row, label, style)
		}
	}
}

func activityProgress(items []agent.PlanItem) (done, active int) {
	active = -1
	for index, item := range items {
		switch item.Status {
		case agent.PlanDone:
			done++
		case agent.PlanActive:
			active = index
		}
	}
	return done, active
}

func activityWindow(total, capacity, active int) (start, end int) {
	if total <= 0 || capacity <= 0 {
		return 0, 0
	}
	capacity = min(capacity, total)
	if active < 0 {
		return 0, capacity
	}
	start = active - capacity/2
	start = min(max(start, 0), total-capacity)
	return start, start + capacity
}

type statusView struct {
	theme   kit.Theme
	glyphs  kit.Glyphs
	doing   string
	elapsed string
	usage   agent.Usage
	outcome agent.Outcome
	status  kit.Status
	busy    bool
	options agent.RunOptions
}

func newStatusView(theme kit.Theme, glyphs kit.Glyphs, options agent.RunOptions) *statusView {
	return &statusView{theme: theme, glyphs: glyphs, doing: "ready", options: options}
}

func (s *statusView) Reset(options agent.RunOptions) {
	theme, glyphs := s.theme, s.glyphs
	*s = statusView{theme: theme, glyphs: glyphs, doing: "ready", options: options}
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
	if right == "" {
		right = optionsLabel(s.options)
	}
	style := s.theme.Muted
	switch s.outcome.Status {
	case agent.OutcomeCompleted:
		style = s.theme.Success
	case agent.OutcomeCanceled:
		style = s.theme.Warning
	case agent.OutcomeFailed:
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

func (s *statusView) setOptions(options agent.RunOptions) { s.options = options }

func optionsLabel(options agent.RunOptions) string {
	parts := make([]string, 0, 4)
	parts = append(parts, modelLabel(options.Model))
	if options.Effort != "" {
		parts = append(parts, options.Effort)
	}
	if options.Mode != "" {
		parts = append(parts, string(options.Mode))
	}
	if options.Permission != "" {
		parts = append(parts, string(options.Permission))
	}
	return strings.Join(parts, " · ")
}

func modelLabel(model string) string {
	if model = strings.TrimSpace(model); model != "" {
		return model
	}
	return "runtime default"
}

func (s *statusView) tick(elapsed time.Duration) {
	s.status.Tick()
	s.elapsed = fmt.Sprintf("%4.1fs", elapsed.Seconds())
}

func (s *statusView) settled(outcome agent.Outcome, usage agent.Usage) {
	s.outcome, s.usage, s.elapsed = outcome, usage, ""
	s.busy = false
	switch outcome.Status {
	case agent.OutcomeCompleted:
		s.doing = "complete"
	case agent.OutcomeCanceled:
		s.doing = "canceled"
	case agent.OutcomeFailed:
		s.doing = "failed: " + outcome.Error
	default:
		s.doing = "ready"
	}
}

func (s *statusView) active(label string) {
	s.doing = label
	s.outcome = agent.Outcome{}
	s.elapsed = ""
	s.busy = true
}

func (s *statusView) note(label string) {
	s.doing = label
	s.outcome = agent.Outcome{}
	s.busy = false
}

func usageLabel(usage agent.Usage) string {
	if usage == (agent.Usage{}) {
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
	digits := strconv.FormatInt(value, 10)
	first := 0
	if digits[0] == '-' {
		first = 1
	}
	for i := len(digits) - 3; i > first; i -= 3 {
		digits = digits[:i] + "," + digits[i:]
	}
	return digits
}

func compactTime(duration time.Duration) string {
	if duration < time.Second {
		return strconv.FormatInt(duration.Milliseconds(), 10) + "ms"
	}
	return strconv.FormatFloat(duration.Seconds(), 'f', 1, 64) + "s"
}
