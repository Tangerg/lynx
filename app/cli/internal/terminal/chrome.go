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
	"github.com/Tangerg/lynx/app/cli/internal/workspace"
)

const (
	headerMinWidth   = 32
	activityMinWidth = 28
	activityMaxRows  = 6
)

type sessionHeader struct {
	theme        kit.Theme
	glyphs       kit.Glyphs
	session      agent.Session
	usage        agent.Usage
	changes      int
	changesKnown bool
}

func newSessionHeader(theme kit.Theme, glyphs kit.Glyphs, session agent.Session) *sessionHeader {
	return &sessionHeader{theme: theme, glyphs: glyphs, session: session}
}

func (h *sessionHeader) SetSession(session agent.Session) { h.session = session }

func (h *sessionHeader) SetUsage(usage agent.Usage) { h.usage = usage.Clone() }

func (h *sessionHeader) SetWorkspaceChanges(count int) {
	h.changes, h.changesKnown = max(count, 0), true
}

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
	right := headerRightLabel(h.usage, h.changes, h.changesKnown)
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

func headerRightLabel(usage agent.Usage, changes int, known bool) string {
	parts := make([]string, 0, 2)
	if tokens := headerUsageLabel(usage); tokens != "" {
		parts = append(parts, tokens)
	}
	if known && changes > 0 {
		parts = append(parts, fmt.Sprintf("Δ%d", changes))
	}
	return strings.Join(parts, "  ")
}

func displayWorkspace(value workspace.Workspace) string {
	path := filepath.Clean(strings.TrimSpace(value.Path))
	if path == "." || path == "" {
		return "workspace"
	}
	if !value.IsAvailable() {
		return path + "  ·  missing"
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
	magnitude := float64(tokens)
	if magnitude < 0 {
		magnitude = -magnitude
	}
	switch {
	case magnitude < 1_000:
		return strconv.FormatInt(tokens, 10)
	case magnitude < 10_000:
		return strconv.FormatFloat(float64(tokens)/1_000, 'f', 1, 64) + "k"
	case magnitude < 1_000_000:
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
	problem string
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
	theme, glyphs, problem := s.theme, s.glyphs, s.problem
	*s = statusView{theme: theme, glyphs: glyphs, doing: "ready", problem: problem, options: options}
}

func (s *statusView) Measure(int) int { return 1 }

func (s *statusView) Draw(view grid.View) {
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	if s.problem != "" {
		kit.Label{Text: s.problem, Style: s.theme.Danger, Ellipsis: s.glyphs.Ellipsis}.Draw(view)
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
	case agent.OutcomeCanceled, agent.OutcomeTimedOut, agent.OutcomeMaxSteps, agent.OutcomeMaxBudget:
		style = s.theme.Warning
	case agent.OutcomeFailed, agent.OutcomeLost:
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

func (s *statusView) setProblem(problem string) { s.problem = strings.TrimSpace(problem) }

func optionsLabel(options agent.RunOptions) string {
	parts := []string{modelLabel(options)}
	if limits := limitsLabel(options.Limits); limits != "" {
		parts = append(parts, strings.TrimPrefix(limits, "\n"))
	}
	return strings.Join(parts, " · ")
}

func modelLabel(options agent.RunOptions) string {
	if options.Provider != "" && options.Model != "" {
		return options.Provider + "/" + options.Model
	}
	return "runtime default"
}

func limitsLabel(limits agent.RunLimits) string {
	parts := make([]string, 0, 3)
	if limits.MaxTotalTokens > 0 {
		parts = append(parts, fmt.Sprintf("tokens ≤ %d", limits.MaxTotalTokens))
	}
	if limits.MaxSteps > 0 {
		parts = append(parts, fmt.Sprintf("steps ≤ %d", limits.MaxSteps))
	}
	if limits.MaxBudgetUSD > 0 {
		parts = append(parts, fmt.Sprintf("budget ≤ $%.2f", limits.MaxBudgetUSD))
	}
	if len(parts) == 0 {
		return ""
	}
	return "\nlimits: " + strings.Join(parts, ", ")
}

func (s *statusView) tick(elapsed time.Duration) {
	s.status.Tick()
	s.elapsed = fmt.Sprintf("%4.1fs", elapsed.Seconds())
}

func (s *statusView) settled(outcome agent.Outcome, usage agent.Usage) {
	s.outcome, s.usage, s.elapsed = outcome, usage.Clone(), ""
	s.busy = false
	switch outcome.Status {
	case agent.OutcomeCompleted:
		s.doing = "complete"
	case agent.OutcomeCanceled:
		s.doing = "canceled"
	case agent.OutcomeTimedOut:
		s.doing = "timed out"
	case agent.OutcomeMaxSteps:
		s.doing = "max steps"
	case agent.OutcomeMaxBudget:
		s.doing = "max budget"
	case agent.OutcomeFailed:
		s.doing = "failed: " + outcome.Description()
	case agent.OutcomeLost:
		s.doing = "lost: " + outcome.Description()
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

func (s *statusView) progress(progress agent.RunProgress) {
	label := strings.TrimSpace(progress.Activity)
	if label == "" {
		label = "working"
	}
	parts := []string{label}
	if progress.Step != nil {
		parts = append(parts, "step "+strconv.Itoa(*progress.Step))
	}
	if progress.ContextTokens != nil {
		parts = append(parts, "ctx "+formatThousands(*progress.ContextTokens))
	}
	s.active(strings.Join(parts, " · "))
}

func (s *statusView) note(label string) {
	s.doing = label
	s.outcome = agent.Outcome{}
	s.busy = false
}

func usageLabel(usage agent.Usage) string {
	if usage.Empty() {
		return ""
	}
	parts := []string{"↑" + formatThousands(usage.InputTokens), "↓" + formatThousands(usage.OutputTokens)}
	if usage.CacheReadTokens > 0 {
		parts = append(parts, "cached "+formatThousands(usage.CacheReadTokens))
	}
	if usage.CostUSD != nil {
		parts = append(parts, "$"+strconv.FormatFloat(*usage.CostUSD, 'f', 4, 64))
	}
	if usage.Steps > 0 {
		parts = append(parts, "steps "+strconv.Itoa(usage.Steps))
	}
	if usage.Duration > 0 {
		parts = append(parts, formatCompactDuration(usage.Duration))
	}
	return strings.Join(parts, "  ")
}

func formatThousands(value int64) string {
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

func formatCompactDuration(duration time.Duration) string {
	if duration < time.Second {
		return strconv.FormatInt(duration.Milliseconds(), 10) + "ms"
	}
	return strconv.FormatFloat(duration.Seconds(), 'f', 1, 64) + "s"
}
