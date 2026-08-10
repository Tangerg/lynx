package terminal

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	coreDiff "github.com/Tangerg/oolong/core/diff"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
	"github.com/Tangerg/oolong/highlight"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

const (
	maxToolDetailLines = 200
	toolContentInset   = 2
)

// mutableToolBlock is the optional lifecycle a block presenter may implement to
// update a running tool in place and participate in the global detail toggle.
// It remains a terminal-only protocol; the domain only knows ToolCall values.
type mutableToolBlock interface {
	headless.Block
	Update(agent.Block)
	AppendOutput(string)
	Finish(agent.ToolStatus)
	SetExpanded(bool)
	Expandable() bool
	Expanded() bool
	ToggleExpanded() bool
}

type toolBlock struct {
	theme    kit.Theme
	glyphs   kit.Glyphs
	syntax   highlight.Style
	call     agent.ToolCall
	expanded bool
	body     []headless.Block
}

var (
	_ headless.Block    = (*toolBlock)(nil)
	_ headless.Copyable = (*toolBlock)(nil)
	_ mutableToolBlock  = (*toolBlock)(nil)
)

func newToolBlock(p Presentation, block agent.Block) *toolBlock {
	t := &toolBlock{theme: p.Theme, glyphs: p.Glyphs, syntax: p.Syntax}
	t.Update(block)
	return t
}

func (t *toolBlock) Update(block agent.Block) {
	if block.Tool == nil {
		t.call = agent.ToolCall{Kind: agent.ToolUnknown, Name: "invalid tool", Summary: "runtime omitted the tool projection", Status: agent.ToolError}
	} else {
		t.call = *block.Tool
		if block.Tool.ExitCode != nil {
			code := *block.Tool.ExitCode
			t.call.ExitCode = &code
		}
	}
	t.rebuild()
}

func (t *toolBlock) AppendOutput(chunk string) {
	if chunk == "" {
		return
	}
	t.call.Output += chunk
	t.rebuild()
}

func (t *toolBlock) Finish(status agent.ToolStatus) {
	if t.call.Status != agent.ToolRunning {
		return
	}
	t.call.Status = status
	t.rebuild()
}

func (t *toolBlock) SetExpanded(expanded bool) { t.expanded = expanded && t.Expandable() }

func (t *toolBlock) Expandable() bool { return t.call.Status == agent.ToolRunning || len(t.body) > 0 }

func (t *toolBlock) Expanded() bool { return t.expanded && t.Expandable() }

func (t *toolBlock) ToggleExpanded() bool {
	if !t.Expandable() {
		t.expanded = false
		return false
	}
	t.expanded = !t.expanded
	return t.expanded
}

func (t *toolBlock) Measure(width int) int {
	rows := 1
	if t.Expanded() {
		for _, block := range t.body {
			rows = layout.Sum(rows, block.Measure(max(width-toolContentInset, 1)))
		}
	}
	return layout.Sum(rows, 1)
}

func (t *toolBlock) Draw(view grid.View) {
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	toggle, label, right, statusStyle := t.header()
	contentRows := min(t.Measure(width)-1, height)
	for row := range contentRows {
		view.Text(0, row, t.glyphs.Vertical, statusStyle)
	}

	contentWidth := max(width-toolContentInset, 0)
	if contentWidth <= 0 {
		return
	}
	toggleStyle := t.theme.Muted
	if t.Expanded() {
		toggleStyle = t.theme.Accent
	}
	view.Text(toolContentInset, 0, toggle, toggleStyle)
	labelX := toolContentInset + text.Width(toggle) + 1
	labelLimit := width
	if right != "" && text.Width(right)+text.Width(toggle)+2 < contentWidth {
		rightWidth := text.Width(right)
		view.Text(width-rightWidth, 0, right, statusStyle)
		labelLimit = width - rightWidth - 1
	}
	if labelWidth := labelLimit - labelX; labelWidth > 0 {
		view.Text(labelX, 0, text.Truncate(label, labelWidth, t.glyphs.Ellipsis), t.theme.Text)
	}
	if !t.Expanded() {
		return
	}
	y, bodyWidth := 1, max(width-toolContentInset, 0)
	for _, block := range t.body {
		rows := block.Measure(max(bodyWidth, 1))
		if y >= height {
			return
		}
		block.Draw(view.Sub(grid.Rect(toolContentInset, y, bodyWidth, min(rows, height-y))))
		y += rows
	}
}

func (t *toolBlock) Rows(width int) []text.Row {
	toggle, label, right, _ := t.header()
	left := toggle + " " + label
	header := strings.TrimSpace(left + " " + right)
	rows := []text.Row{{Text: header}}
	if t.Expanded() {
		bodyWidth := max(width-toolContentInset, 1)
		for _, block := range t.body {
			height := block.Measure(bodyWidth)
			if copyable, ok := block.(headless.Copyable); ok {
				copied := copyable.Rows(bodyWidth)
				for i := range min(len(copied), height) {
					copied[i].Offset += toolContentInset
				}
				rows = append(rows, copied[:min(len(copied), height)]...)
				for range height - min(len(copied), height) {
					rows = append(rows, text.Row{})
				}
				continue
			}
			rows = append(rows, make([]text.Row, height)...)
		}
	}
	return append(rows, text.Row{})
}

func (t *toolBlock) header() (toggle, label, status string, statusStyle grid.Style) {
	toggle = t.glyphs.Bullet
	if t.Expandable() {
		toggle = t.glyphs.Collapsed
	}
	if t.Expanded() {
		toggle = t.glyphs.Expanded
	}
	label = toolLabel(t.call)
	statusStyle = t.theme.Muted
	switch t.call.Status {
	case agent.ToolRunning:
		status = t.glyphs.Marker + " running"
		statusStyle = t.theme.Info
	case agent.ToolOK:
		status = t.glyphs.Taken + " done"
		statusStyle = t.theme.Success
	case agent.ToolError:
		status = t.glyphs.Taken + " error"
		statusStyle = t.theme.Danger
	case agent.ToolCanceled:
		status = t.glyphs.Bullet + " canceled"
		statusStyle = t.theme.Warning
	default:
		status = string(t.call.Status)
	}
	if t.call.ExitCode != nil && *t.call.ExitCode != 0 {
		status += " exit " + strconv.Itoa(*t.call.ExitCode)
	}
	if t.call.Duration > 0 {
		status += " " + compactTime(t.call.Duration)
	}
	return toggle, label, strings.TrimSpace(status), statusStyle
}

func toolLabel(call agent.ToolCall) string {
	switch call.Kind {
	case agent.ToolShell:
		return shellToolLabel(call)
	case agent.ToolEdit:
		return toolKindLabel("edit", toolPrimary(call.Path, call.Summary))
	case agent.ToolRead:
		return toolKindLabel("read", toolPrimary(call.Path, call.Summary))
	case agent.ToolSearch:
		return toolKindLabel("search", toolPrimary(call.Query, call.Summary))
	case agent.ToolWeb:
		return toolKindLabel("web", toolPrimary(call.URL, call.Summary))
	case agent.ToolTask:
		return toolKindLabel("task", strings.TrimSpace(call.Summary))
	case agent.ToolUnknown:
		return unknownToolLabel(call)
	default:
		return unknownToolLabel(call)
	}
}

func shellToolLabel(call agent.ToolCall) string {
	primary := toolPrimary(call.Command, call.Summary)
	if primary == "" {
		return "shell"
	}
	return "$ " + primary
}

func unknownToolLabel(call agent.ToolCall) string {
	name := strings.TrimSpace(call.Name)
	if name == "" {
		name = "tool"
	}
	return toolKindLabel(name, strings.TrimSpace(call.Summary))
}

func toolPrimary(specific, fallback string) string {
	if specific = strings.TrimSpace(specific); specific != "" {
		return specific
	}
	return strings.TrimSpace(fallback)
}

func toolKindLabel(kind, primary string) string {
	if primary == "" {
		return kind
	}
	return kind + " · " + primary
}

func (t *toolBlock) rebuild() {
	t.body = nil
	output := truncateToolDetail(t.call.Output)
	if t.call.Diff != "" {
		if hunks := parseUnifiedDiff(t.call.Diff); len(hunks) > 0 {
			change := kit.NewDiff(t.theme, t.glyphs, hunks)
			change.ShowNumbers(true)
			t.body = append(t.body, change)
		} else {
			t.body = append(t.body, kit.NewCode(highlight.Lines("diff", truncateToolDetail(t.call.Diff), t.syntax)))
		}
	}
	if output == "" {
		return
	}
	switch t.call.Kind {
	case agent.ToolRead:
		code := kit.NewCode(highlight.Lines(languageForPath(t.call.Path), output, t.syntax))
		code.Gutter = kit.LineNumbers{Style: t.theme.Subtle, Separator: t.glyphs.Vertical}
		t.body = append(t.body, code)
	case agent.ToolSearch, agent.ToolWeb, agent.ToolTask:
		paragraph := kit.NewParagraph(output, t.theme.Text)
		paragraph.Links = t.call.Kind == agent.ToolWeb
		t.body = append(t.body, paragraph)
	case agent.ToolUnknown, agent.ToolShell, agent.ToolEdit:
		t.body = append(t.body, kit.NewCode(highlight.Lines("text", output, t.syntax)))
	default:
		t.body = append(t.body, kit.NewCode(highlight.Lines("text", output, t.syntax)))
	}
}

func truncateToolDetail(value string) string {
	lines := strings.Split(strings.TrimRight(value, "\n"), "\n")
	if len(lines) <= maxToolDetailLines {
		return strings.Join(lines, "\n")
	}
	head, tail := 140, 40
	omitted := len(lines) - head - tail
	kept := append([]string(nil), lines[:head]...)
	kept = append(kept, "… "+strconv.Itoa(omitted)+" lines omitted …")
	kept = append(kept, lines[len(lines)-tail:]...)
	return strings.Join(kept, "\n")
}

func languageForPath(path string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	switch ext {
	case "yml":
		return "yaml"
	case "md":
		return "markdown"
	case "sh", "bash", "zsh":
		return "shell"
	case "js", "jsx":
		return "javascript"
	case "ts", "tsx":
		return "typescript"
	case "py":
		return "python"
	case "rs":
		return "rust"
	case "go", "json", "yaml", "toml", "xml", "html", "css", "sql":
		return ext
	default:
		return "text"
	}
}

func parseUnifiedDiff(patch string) []coreDiff.Hunk {
	var hunks []coreDiff.Hunk
	var current *coreDiff.Hunk
	oldLine, newLine := 0, 0
	for line := range strings.SplitSeq(strings.TrimRight(patch, "\n"), "\n") {
		if strings.HasPrefix(line, "@@ ") {
			oldStart, newStart, ok := parseHunkHeader(line)
			if !ok {
				current = nil
				continue
			}
			hunks = append(hunks, coreDiff.Hunk{Old: oldStart, New: newStart})
			current = &hunks[len(hunks)-1]
			oldLine, newLine = oldStart, newStart
			continue
		}
		if current == nil || line == "" {
			continue
		}
		body := line[1:]
		switch line[0] {
		case ' ':
			current.Lines = append(current.Lines, coreDiff.Line{Kind: coreDiff.Context, Text: body, Old: oldLine, New: newLine})
			oldLine, newLine = oldLine+1, newLine+1
		case '-':
			current.Lines = append(current.Lines, coreDiff.Line{Kind: coreDiff.Removed, Text: body, Old: oldLine})
			oldLine++
		case '+':
			current.Lines = append(current.Lines, coreDiff.Line{Kind: coreDiff.Added, Text: body, New: newLine})
			newLine++
		}
	}
	return hunks
}

func parseHunkHeader(line string) (oldStart, newStart int, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 || !strings.HasPrefix(fields[1], "-") || !strings.HasPrefix(fields[2], "+") {
		return 0, 0, false
	}
	parseStart := func(value string) (int, bool) {
		value = strings.TrimLeft(value, "+-")
		if before, _, found := strings.Cut(value, ","); found {
			value = before
		}
		start, err := strconv.Atoi(value)
		return start, err == nil && start >= 0
	}
	oldStart, oldOK := parseStart(fields[1])
	newStart, newOK := parseStart(fields[2])
	return oldStart, newStart, oldOK && newOK
}
