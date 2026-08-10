package terminal

import (
	"fmt"
	"image"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

func (q *queueDrawer) Place(space image.Point) layout.Placement {
	innerRows := min(max(len(q.snapshot.Entries), 1), queueDrawerVisibleRows)
	if q.Editing() {
		innerRows = 6
	} else if entry, ok := q.selectedEntry(); ok {
		lines := strings.Split(entry.Message.Text, "\n")
		if len(lines) > 1 {
			innerRows += min(len(lines), 3) + 1
		}
	}
	return layout.Placement{Anchor: layout.Bottom, Height: min(innerRows+2, max(space.Y-2, 1)), Margin: 1}
}

func (q *queueDrawer) Draw(frame headless.Frame) {
	width, height := frame.Size()
	if width <= 0 || height <= 0 {
		q.presentation.Stage(frame, queuePresentation{})
		q.editorRegion.Clear(frame)
		return
	}
	box := kit.Box{
		Theme: q.theme, Glyphs: q.glyphs, Padding: layout.Symmetric(0, 1),
		Title: q.title(), Footer: q.footer(), FooterAlign: layout.End,
	}
	inner := box.InnerRect(frame.Bounds().Size())
	box.Draw(frame.View)
	if q.Editing() {
		editorArea := q.drawEditor(frame, inner)
		q.presentation.Stage(frame, queuePresentation{editorArea: editorArea})
		return
	}
	q.editorRegion.Clear(frame)
	hits, rowRows := q.drawEntries(frame.View.Sub(inner))
	for index := range hits {
		hits[index].area = hits[index].area.Add(inner.Min)
	}
	q.presentation.Stage(frame, queuePresentation{hits: slices.Clone(hits), rowRows: rowRows})
}

func (q *queueDrawer) title() string {
	return fmt.Sprintf("Queue · %s", countedNoun(len(q.snapshot.Entries), "prompt"))
}

func (q *queueDrawer) footer() string {
	if q.notice != "" {
		return q.notice
	}
	if q.Editing() {
		return "enter save · shift/alt+enter newline · ctrl+enter send now · esc discard"
	}
	return "j/k move · enter edit · x remove · J/K reorder · s send now · esc close"
}

func (q *queueDrawer) drawEditor(frame headless.Frame, inner image.Rectangle) image.Rectangle {
	entry, ok := q.entry(q.editingID)
	if !ok || inner.Empty() {
		q.editorRegion.Clear(frame)
		return image.Rectangle{}
	}
	rows := layout.Down.Rects(inner.Size(),
		layout.Slot{Size: layout.Fixed(1)},
		layout.Slot{Size: layout.Flex(1)},
	)
	header, field := rows[0].Add(inner.Min), rows[1].Add(inner.Min)
	label := "Editing queued prompt"
	if attachments := len(entry.Message.Attachments); attachments > 0 {
		label += " · keeps " + countedNoun(attachments, "attachment")
	}
	frame.Text(header.Min.X, header.Min.Y, text.Truncate(label, header.Dx(), q.glyphs.Ellipsis), q.theme.Heading)
	q.editorRegion.Stage(frame, field, &q.editor)
	q.editor.Draw(frame.Sub(field))
	return field
}

func (q *queueDrawer) drawEntries(view grid.View) ([]queueHit, int) {
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return nil, 0
	}
	entries := q.snapshot.Entries
	if len(entries) == 0 {
		view.Text(0, 0, "No queued prompts.", q.theme.Muted)
		return nil, 0
	}
	hits := make([]queueHit, 0, queueDrawerVisibleRows*4)
	y := 0
	if selected, ok := q.selectedEntry(); ok {
		lines := strings.Split(selected.Message.Text, "\n")
		if len(lines) > 1 && height >= 4 {
			view.Text(0, y, "Preview · full queued prompt", q.theme.Subtle.Merge(grid.Style{Attr: grid.Bold}))
			y++
			for _, line := range lines[:min(len(lines), 3)] {
				if y >= height-1 {
					break
				}
				view.Text(2, y, text.Truncate(cleanQueueText(line), max(width-2, 1), q.glyphs.Ellipsis), q.theme.Accent)
				y++
			}
		}
	}
	rowRows := min(queueDrawerVisibleRows, max(height-y, 1))
	viewport := visibleQueueStart(q.viewport, q.selected, rowRows, len(entries))
	end := min(viewport+rowRows, len(entries))
	for index := viewport; index < end; index++ {
		entry := entries[index]
		rowY := y + index - viewport
		if rowY >= height {
			break
		}
		row := grid.Rect(0, rowY, width, 1)
		rowTarget := queueTarget{kind: queueTargetRow, id: entry.ID}
		style := q.theme.Text
		if index == q.selected || q.hovered.id == entry.ID {
			style = style.Merge(q.theme.Selection)
			view.Fill(row, q.theme.Selection)
		}
		if q.pressed == rowTarget {
			style = style.Merge(grid.Style{Attr: grid.Bold | grid.Reverse})
		}
		hits = append(hits, queueHit{area: row, target: rowTarget})

		marker := " "
		if index == q.selected {
			marker = q.glyphs.Marker
		}
		prefix := fmt.Sprintf("%s %d. ", marker, index+1)
		right := width
		if width >= 40 && (index == q.selected || q.hovered.id == entry.ID || q.pressed.id == entry.ID) {
			right = q.drawActions(view, row, entry.ID, right, style, &hits)
		}
		view.Text(0, rowY, prefix, style.Merge(q.theme.Muted))
		left := text.Width(prefix)
		label := queueEntryLabel(entry)
		view.Text(left, rowY, text.Truncate(label, max(right-left, 1), q.glyphs.Ellipsis), style)
	}
	return hits, rowRows
}

func (q *queueDrawer) drawActions(view grid.View, row image.Rectangle, id uint64, right int, rowStyle grid.Style, hits *[]queueHit) int {
	buttons := []struct {
		label  string
		target queueTarget
		style  grid.Style
	}{
		{label: "[remove]", target: queueTarget{kind: queueTargetRemove, id: id}, style: q.theme.Danger},
		{label: "[edit]", target: queueTarget{kind: queueTargetEdit, id: id}, style: q.theme.Accent},
		{label: "[send now]", target: queueTarget{kind: queueTargetSend, id: id}, style: q.theme.Success},
	}
	for _, button := range buttons {
		buttonWidth := text.Width(button.label)
		x := right - buttonWidth
		if x < 8 {
			continue
		}
		style := rowStyle.Merge(q.theme.Muted)
		if q.hovered == button.target {
			style = style.Merge(button.style)
		}
		if q.pressed == button.target {
			style = style.Merge(grid.Style{Attr: grid.Bold | grid.Reverse})
		}
		view.Text(x, row.Min.Y, button.label, style)
		area := grid.Rect(x, row.Min.Y, buttonWidth, 1)
		*hits = append(*hits, queueHit{area: area, target: button.target})
		right = x
	}
	return right
}

func cleanQueueText(value string) string {
	return strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return ' '
		}
		return r
	}, value)
}
