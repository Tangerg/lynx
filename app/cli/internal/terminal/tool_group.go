package terminal

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"

	"github.com/Tangerg/scope/app/cli/internal/agent"
)

// toolGroupBlock is one disclosure for adjacent resource-inspection calls.
// Each child remains an ordinary toolBlock and therefore retains its own
// lifecycle, presenter, output, reader sections, and failure status.
type toolGroupBlock struct {
	theme        kit.Theme
	glyphs       kit.Glyphs
	tools        []*toolBlock
	expanded     bool
	open         bool
	nextObserver uint64
	observers    map[uint64]func(readerDocument)
}

var (
	_ headless.Block       = (*toolGroupBlock)(nil)
	_ headless.Copyable    = (*toolGroupBlock)(nil)
	_ toolDisclosure       = (*toolGroupBlock)(nil)
	_ readerDocumentSource = (*toolGroupBlock)(nil)
)

func newToolGroupBlock(theme kit.Theme, glyphs kit.Glyphs, expanded bool) *toolGroupBlock {
	return &toolGroupBlock{theme: theme, glyphs: glyphs, expanded: expanded, open: true}
}

func (t *toolGroupBlock) Add(tool *toolBlock) {
	if t == nil || tool == nil {
		return
	}
	t.tools = append(t.tools, tool)
	tool.SetExpanded(t.expanded)
	tool.Observe(func(readerDocument) { t.notify() })
	t.notify()
}

func (t *toolGroupBlock) Seal() {
	if t == nil || !t.open {
		return
	}
	t.open = false
	t.notify()
}

func (t *toolGroupBlock) ReadyToFinish() bool {
	if t == nil || t.open || len(t.tools) == 0 {
		return false
	}
	for _, tool := range t.tools {
		if tool.call.Status == agent.ToolRunning {
			return false
		}
	}
	return true
}

func (t *toolGroupBlock) SetExpanded(expanded bool) {
	if t == nil {
		return
	}
	t.expanded = expanded && t.Expandable()
	for _, tool := range t.tools {
		tool.SetExpanded(t.expanded)
	}
}

func (t *toolGroupBlock) Expandable() bool {
	for _, tool := range t.tools {
		if tool.Expandable() {
			return true
		}
	}
	return false
}

func (t *toolGroupBlock) Expanded() bool { return t != nil && t.expanded && t.Expandable() }

func (t *toolGroupBlock) ToggleExpanded() bool {
	if !t.Expandable() {
		t.SetExpanded(false)
		return false
	}
	t.SetExpanded(!t.Expanded())
	return t.Expanded()
}

func (t *toolGroupBlock) Measure(width int) int {
	if len(t.tools) == 1 {
		return t.tools[0].Measure(width)
	}
	rows := 1
	if t.Expanded() {
		for _, tool := range t.tools {
			rows = layout.Sum(rows, tool.Measure(max(width-toolContentInset, 1)))
		}
	}
	return layout.Sum(rows, 1)
}

func (t *toolGroupBlock) Draw(view grid.View) {
	if len(t.tools) == 0 {
		return
	}
	if len(t.tools) == 1 {
		t.tools[0].Draw(view)
		return
	}
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	toggle, label, status, statusStyle := t.header()
	for row := range min(t.Measure(width)-1, height) {
		view.Text(0, row, t.glyphs.Vertical, statusStyle)
	}
	toggleStyle := t.theme.Muted
	if t.Expanded() {
		toggleStyle = t.theme.Accent
	}
	view.Text(toolContentInset, 0, toggle, toggleStyle)
	labelX := toolContentInset + text.Width(toggle) + 1
	labelLimit := width
	if status != "" && text.Width(status)+labelX+1 < width {
		statusWidth := text.Width(status)
		view.Text(width-statusWidth, 0, status, statusStyle)
		labelLimit = width - statusWidth - 1
	}
	if labelLimit > labelX {
		view.Text(labelX, 0, text.Truncate(label, labelLimit-labelX, t.glyphs.Ellipsis), t.theme.Text)
	}
	if !t.Expanded() {
		return
	}
	y, childWidth := 1, max(width-toolContentInset, 1)
	for _, tool := range t.tools {
		rows := tool.Measure(childWidth)
		if y >= height {
			return
		}
		tool.Draw(view.Sub(grid.Rect(toolContentInset, y, childWidth, min(rows, height-y))))
		y += rows
	}
}

func (t *toolGroupBlock) Rows(width int) []text.Row {
	if len(t.tools) == 0 {
		return nil
	}
	if len(t.tools) == 1 {
		return t.tools[0].Rows(width)
	}
	toggle, label, status, _ := t.header()
	rows := []text.Row{{Text: strings.TrimSpace(toggle + " " + label + " " + status)}}
	if t.Expanded() {
		childWidth := max(width-toolContentInset, 1)
		for _, tool := range t.tools {
			copied := tool.Rows(childWidth)
			for index := range copied {
				copied[index].Offset += toolContentInset
			}
			rows = append(rows, copied...)
		}
	}
	return append(rows, text.Row{})
}

func (t *toolGroupBlock) header() (toggle, label, status string, style grid.Style) {
	toggle = t.glyphs.Bullet
	if t.Expandable() {
		toggle = t.glyphs.Collapsed
	}
	if t.Expanded() {
		toggle = t.glyphs.Expanded
	}
	label = fmt.Sprintf("%d resource operations", len(t.tools))
	running, failed, canceled := 0, 0, 0
	for _, tool := range t.tools {
		switch tool.call.Status {
		case agent.ToolRunning:
			running++
		case agent.ToolError:
			failed++
		case agent.ToolCanceled:
			canceled++
		}
	}
	style = t.theme.Success
	switch {
	case failed > 0:
		status, style = countedNoun(failed, "error"), t.theme.Danger
	case running > 0:
		status, style = fmt.Sprintf("%d running", running), t.theme.Info
	case canceled > 0:
		status, style = fmt.Sprintf("%d canceled", canceled), t.theme.Warning
	default:
		status = fmt.Sprintf("%d done", len(t.tools))
	}
	return toggle, label, status, style
}

func (t *toolGroupBlock) readerDocument() readerDocument {
	if len(t.tools) == 1 {
		return t.tools[0].readerDocument()
	}
	_, title, detail, _ := t.header()
	document := readerDocument{Title: title, Detail: detail}
	for index, tool := range t.tools {
		child := tool.readerDocument()
		prefix := fmt.Sprintf("%d. %s", index+1, child.Title)
		document.Sections = append(document.Sections, ToolSection{
			Title: prefix, Style: toolSectionParagraph, Text: child.Detail,
		})
		for _, section := range child.Sections {
			section.Title = prefix + " · " + section.Title
			document.Sections = append(document.Sections, section)
		}
	}
	return document
}

func (t *toolGroupBlock) Observe(observer func(readerDocument)) func() {
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

func (t *toolGroupBlock) notify() {
	if t == nil {
		return
	}
	document := t.readerDocument()
	for _, observer := range t.observers {
		observer(document)
	}
}

func groupableTool(call agent.ToolCall) bool {
	return slices.Contains([]agent.ToolKind{agent.ToolRead, agent.ToolSearch, agent.ToolWeb}, call.Kind)
}
