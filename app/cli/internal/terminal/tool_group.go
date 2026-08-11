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

	"github.com/Tangerg/lynx/app/cli/internal/agent"
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

func (g *toolGroupBlock) Add(tool *toolBlock) {
	if g == nil || tool == nil {
		return
	}
	g.tools = append(g.tools, tool)
	tool.SetExpanded(g.expanded)
	tool.Observe(func(readerDocument) { g.notify() })
	g.notify()
}

func (g *toolGroupBlock) Seal() {
	if g == nil || !g.open {
		return
	}
	g.open = false
	g.notify()
}

func (g *toolGroupBlock) ReadyToFinish() bool {
	if g == nil || g.open || len(g.tools) == 0 {
		return false
	}
	for _, tool := range g.tools {
		if tool.call.Status == agent.ToolRunning {
			return false
		}
	}
	return true
}

func (g *toolGroupBlock) SetExpanded(expanded bool) {
	if g == nil {
		return
	}
	g.expanded = expanded && g.Expandable()
	for _, tool := range g.tools {
		tool.SetExpanded(g.expanded)
	}
}

func (g *toolGroupBlock) Expandable() bool {
	for _, tool := range g.tools {
		if tool.Expandable() {
			return true
		}
	}
	return false
}

func (g *toolGroupBlock) Expanded() bool { return g != nil && g.expanded && g.Expandable() }

func (g *toolGroupBlock) ToggleExpanded() bool {
	if !g.Expandable() {
		g.SetExpanded(false)
		return false
	}
	g.SetExpanded(!g.Expanded())
	return g.Expanded()
}

func (g *toolGroupBlock) Measure(width int) int {
	if len(g.tools) == 1 {
		return g.tools[0].Measure(width)
	}
	rows := 1
	if g.Expanded() {
		for _, tool := range g.tools {
			rows = layout.Sum(rows, tool.Measure(max(width-toolContentInset, 1)))
		}
	}
	return layout.Sum(rows, 1)
}

func (g *toolGroupBlock) Draw(view grid.View) {
	if len(g.tools) == 0 {
		return
	}
	if len(g.tools) == 1 {
		g.tools[0].Draw(view)
		return
	}
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	toggle, label, status, statusStyle := g.header()
	for row := range min(g.Measure(width)-1, height) {
		view.Text(0, row, g.glyphs.Vertical, statusStyle)
	}
	toggleStyle := g.theme.Muted
	if g.Expanded() {
		toggleStyle = g.theme.Accent
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
		view.Text(labelX, 0, text.Truncate(label, labelLimit-labelX, g.glyphs.Ellipsis), g.theme.Text)
	}
	if !g.Expanded() {
		return
	}
	y, childWidth := 1, max(width-toolContentInset, 1)
	for _, tool := range g.tools {
		rows := tool.Measure(childWidth)
		if y >= height {
			return
		}
		tool.Draw(view.Sub(grid.Rect(toolContentInset, y, childWidth, min(rows, height-y))))
		y += rows
	}
}

func (g *toolGroupBlock) Rows(width int) []text.Row {
	if len(g.tools) == 0 {
		return nil
	}
	if len(g.tools) == 1 {
		return g.tools[0].Rows(width)
	}
	toggle, label, status, _ := g.header()
	rows := []text.Row{{Text: strings.TrimSpace(toggle + " " + label + " " + status)}}
	if g.Expanded() {
		childWidth := max(width-toolContentInset, 1)
		for _, tool := range g.tools {
			copied := tool.Rows(childWidth)
			for index := range copied {
				copied[index].Offset += toolContentInset
			}
			rows = append(rows, copied...)
		}
	}
	return append(rows, text.Row{})
}

func (g *toolGroupBlock) header() (toggle, label, status string, style grid.Style) {
	toggle = g.glyphs.Bullet
	if g.Expandable() {
		toggle = g.glyphs.Collapsed
	}
	if g.Expanded() {
		toggle = g.glyphs.Expanded
	}
	label = fmt.Sprintf("%d resource operations", len(g.tools))
	running, failed, canceled := 0, 0, 0
	for _, tool := range g.tools {
		switch tool.call.Status {
		case agent.ToolRunning:
			running++
		case agent.ToolError:
			failed++
		case agent.ToolCanceled:
			canceled++
		}
	}
	style = g.theme.Success
	switch {
	case failed > 0:
		status, style = countedNoun(failed, "error"), g.theme.Danger
	case running > 0:
		status, style = fmt.Sprintf("%d running", running), g.theme.Info
	case canceled > 0:
		status, style = fmt.Sprintf("%d canceled", canceled), g.theme.Warning
	default:
		status = fmt.Sprintf("%d done", len(g.tools))
	}
	return toggle, label, status, style
}

func (g *toolGroupBlock) readerDocument() readerDocument {
	if len(g.tools) == 1 {
		return g.tools[0].readerDocument()
	}
	_, title, detail, _ := g.header()
	document := readerDocument{Title: title, Detail: detail}
	for index, tool := range g.tools {
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

func (g *toolGroupBlock) Observe(observer func(readerDocument)) func() {
	if g == nil || observer == nil {
		return func() {}
	}
	if g.observers == nil {
		g.observers = make(map[uint64]func(readerDocument))
	}
	g.nextObserver++
	id := g.nextObserver
	g.observers[id] = observer
	observer(g.readerDocument())
	return func() { delete(g.observers, id) }
}

func (g *toolGroupBlock) notify() {
	if g == nil {
		return
	}
	document := g.readerDocument()
	for _, observer := range g.observers {
		observer(document)
	}
}

func groupableTool(call agent.ToolCall) bool {
	return slices.Contains([]agent.ToolKind{agent.ToolRead, agent.ToolSearch, agent.ToolWeb}, call.Kind)
}
