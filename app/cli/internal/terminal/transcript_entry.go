package terminal

import (
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

const transcriptEntryInset = 2

// transcriptEntry gives every retained block the same interaction surface. The
// wrapped block remains responsible for its content; this wrapper owns only the
// stable selection rail used by transcript-focused keyboard navigation.
type transcriptEntry struct {
	theme    kit.Theme
	glyphs   kit.Glyphs
	content  headless.Block
	selected bool
	focused  bool
}

var (
	_ headless.Block    = (*transcriptEntry)(nil)
	_ headless.Copyable = (*transcriptEntry)(nil)
)

func newTranscriptEntry(theme kit.Theme, glyphs kit.Glyphs, content headless.Block) *transcriptEntry {
	return &transcriptEntry{theme: theme, glyphs: glyphs, content: content}
}

func (e *transcriptEntry) Measure(width int) int {
	if e == nil || e.content == nil {
		return 0
	}
	contentWidth, _ := e.geometry(width)
	return e.content.Measure(contentWidth)
}

func (e *transcriptEntry) Draw(view grid.View) {
	if e == nil || e.content == nil {
		return
	}
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	contentWidth, inset := e.geometry(width)
	if inset == 0 {
		e.content.Draw(view)
		return
	}
	e.content.Draw(view.Sub(grid.Rect(inset, 0, contentWidth, height)))
	if !e.selected {
		return
	}
	style := e.theme.Subtle
	if e.focused {
		style = e.theme.Accent
	}
	for row := range height {
		view.Text(0, row, e.glyphs.Vertical, style)
	}
}

func (e *transcriptEntry) Rows(width int) []text.Row {
	if e == nil || e.content == nil {
		return nil
	}
	height := e.Measure(width)
	rows := make([]text.Row, height)
	copyable, ok := e.content.(headless.Copyable)
	if !ok {
		return rows
	}
	contentWidth, inset := e.geometry(width)
	copied := copyable.Rows(contentWidth)
	for index := range min(height, len(copied)) {
		rows[index] = copied[index]
		rows[index].Offset = layout.Sum(rows[index].Offset, inset)
	}
	return rows
}

func (e *transcriptEntry) geometry(width int) (contentWidth, inset int) {
	if width <= transcriptEntryInset {
		return max(width, 1), 0
	}
	return width - transcriptEntryInset, transcriptEntryInset
}

func copyableRowsText(rows []text.Row) string {
	var copied strings.Builder
	for index, row := range rows {
		if index > 0 {
			copied.WriteString(row.Separator())
		}
		copied.WriteString(strings.TrimRight(row.Text, " "))
	}
	return strings.TrimRight(copied.String(), "\n")
}
