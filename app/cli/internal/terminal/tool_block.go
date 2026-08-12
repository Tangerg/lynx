package terminal

import (
	"path/filepath"
	"slices"
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

// toolDisclosure is the shared interaction contract for an individual tool or
// a semantic group of adjacent tools.
type toolDisclosure interface {
	headless.Block
	SetExpanded(bool)
	Expandable() bool
	Expanded() bool
	ToggleExpanded() bool
}

// mutableToolBlock is the optional lifecycle a block presenter may implement to
// update a running tool in place. It remains a terminal-only protocol; the
// domain only knows ToolCall values.
type mutableToolBlock interface {
	toolDisclosure
	Update(agent.Block)
	AppendOutput(string)
	Finish(agent.ToolStatus)
}

type toolBlock struct {
	theme        kit.Theme
	glyphs       kit.Glyphs
	syntax       highlight.Renderer
	presenters   []ToolPresenter
	call         agent.ToolCall
	presentation ToolPresentation
	expanded     bool
	body         []headless.Block
	nextObserver uint64
	observers    map[uint64]func(readerDocument)
}

var (
	_ headless.Block    = (*toolBlock)(nil)
	_ headless.Copyable = (*toolBlock)(nil)
	_ mutableToolBlock  = (*toolBlock)(nil)
)

func newToolBlock(p BlockPresentation, block agent.Block) *toolBlock {
	t := &toolBlock{theme: p.Theme, glyphs: p.Glyphs, syntax: p.Syntax, presenters: slices.Clone(p.Tools)}
	t.Update(block)
	return t
}

func (t *toolBlock) Update(block agent.Block) {
	if block.Tool == nil {
		t.call = agent.ToolCall{Kind: agent.ToolUnknown, Name: "invalid tool", Summary: "runtime omitted the tool projection", Status: agent.ToolError}
	} else {
		t.call = block.Tool.Clone()
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
	label = t.presentation.Label
	if label == "" {
		label = "tool"
	}
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
		status += " " + formatCompactDuration(t.call.Duration)
	}
	return toggle, label, strings.TrimSpace(status), statusStyle
}

func (t *toolBlock) rebuild() {
	presentation, err := selectToolPresentation(t.presenters, t.call)
	if err != nil {
		presentation = ToolPresentation{
			Label:    unknownToolLabel(t.call),
			Sections: []ToolSection{{Title: "Presentation error", Style: toolSectionCode, Language: "text", Text: err.Error()}},
		}
	}
	t.presentation = presentation
	t.body = renderToolSections(BlockPresentation{Theme: t.theme, Glyphs: t.glyphs, Syntax: t.syntax}, presentation.Sections, true)
	for _, observer := range t.observers {
		observer(t.readerDocument())
	}
}

func (t *toolBlock) Observe(observer func(readerDocument)) func() {
	if t == nil || observer == nil {
		return func() {}
	}
	if t.observers == nil {
		t.observers = make(map[uint64]func(readerDocument))
	}
	t.nextObserver++
	id := t.nextObserver
	t.observers[id] = observer
	observer(t.readerDocument())
	return func() { delete(t.observers, id) }
}

func (t *toolBlock) readerDocument() readerDocument {
	_, _, status, _ := t.header()
	return readerDocument{Title: t.presentation.Label, Detail: status, Sections: slices.Clone(t.presentation.Sections)}
}

func renderToolSections(p BlockPresentation, sections []ToolSection, truncate bool) []headless.Block {
	blocks := make([]headless.Block, 0, len(sections))
	for _, section := range sections {
		value := section.Text
		if truncate {
			value = truncateToolDetail(value)
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		switch section.Style {
		case toolSectionDiff:
			if hunks := parseUnifiedDiff(value); len(hunks) > 0 {
				change := kit.NewDiff(p.Theme, p.Glyphs, hunks)
				change.ShowNumbers(true)
				blocks = append(blocks, change)
			} else {
				blocks = append(blocks, kit.NewCode(p.Syntax.Lines("diff", value)))
			}
		case toolSectionParagraph:
			paragraph := kit.NewParagraph(value, p.Theme.Text)
			paragraph.SetLinks(kit.LinkConfig{Enabled: section.Links})
			blocks = append(blocks, paragraph)
		case toolSectionCode:
			language := section.Language
			if language == "" {
				language = "text"
			}
			code := kit.NewCode(p.Syntax.Lines(language, value))
			if section.LineNumbers {
				code.Gutter = kit.LineNumbers{Style: p.Theme.Subtle, Separator: p.Glyphs.Vertical}
			}
			blocks = append(blocks, code)
		}
	}
	return blocks
}

func truncateToolDetail(value string) string {
	lines := strings.Split(strings.TrimRight(value, "\n"), "\n")
	if len(lines) <= maxToolDetailLines {
		return strings.Join(lines, "\n")
	}
	head, tail := 140, 40
	omitted := len(lines) - head - tail
	kept := slices.Clone(lines[:head])
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
