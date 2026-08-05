// Package parts is the widgets that mean something here.
//
// An atom is a list; a part is the transcript of a run. The difference is knowledge:
// a part knows what a tool call is, what an approval looks like, and how a plan
// should read. That knowledge has to live somewhere, and gathering it here is what
// keeps the atoms reusable and the screens short.
//
// A part reads the client's view models and the store's folded state. It does not
// fetch anything and it does not arrange screens.
package parts

import (
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/oolong/atoms"
	"github.com/Tangerg/oolong/atoms/theme"
	"github.com/Tangerg/oolong/primitives/grid"
	"github.com/Tangerg/oolong/primitives/input"
	"github.com/Tangerg/oolong/primitives/text"
)

// Transcript draws a conversation and scrolls through it.
//
// The layout is computed once per frame into a flat list of rows, and the visible
// slice of that is drawn. Blocks are of wildly different heights — a one-line notice
// beside a fifty-line diff — so the rows are what scrolling and hit-testing work in;
// scrolling in blocks would jump a screenful at a time.
type Transcript struct {
	Theme theme.Theme
	// MaxToolRows caps how much of a tool's output is shown before it is folded to a
	// summary. Zero means show it all, which a long test run makes unreadable.
	MaxToolRows int

	blocks []client.Block
	// stamp is the store revision the layout was built from, so the layout is rebuilt
	// when the conversation changes and not on every frame.
	stamp  uint64
	built  bool
	width  int
	rows   []transcriptRow
	scroll atoms.Scroll
	keys   atoms.ScrollKeys
}

// transcriptRow is one drawn row: a line of text, and the block it came from.
type transcriptRow struct {
	line text.Line
	// block is the index of the block this row belongs to, for a hit test.
	block int
}

// NewTranscript returns a transcript that follows the conversation as it grows.
func NewTranscript(t theme.Theme) *Transcript {
	tr := &Transcript{Theme: t, MaxToolRows: 12, keys: atoms.DefaultScrollKeys()}
	tr.scroll.ToBottom()
	return tr
}

// Update gives the transcript the conversation to draw. The revision decides whether
// the layout has to be rebuilt, so an idle frame costs nothing.
func (t *Transcript) Update(blocks []client.Block, revision uint64) {
	if t.built && revision == t.stamp {
		return
	}
	t.blocks, t.stamp, t.built = blocks, revision, false
}

// Scroll exposes the position, for a scrollbar drawn beside the transcript.
func (t *Transcript) Scroll() *atoms.Scroll { return &t.scroll }

// Handle scrolls, reporting whether it consumed the event.
func (t *Transcript) Handle(ev input.Event) bool { return t.scroll.Handle(ev, t.keys) }

// Height is how tall the whole conversation is at a width, which a container needs
// when the transcript shares a pane with something else.
func (t *Transcript) Height(width int) int { return len(t.layout(width)) }

// Draw paints the visible part of the conversation.
func (t *Transcript) Draw(v grid.View) {
	width, height := v.Size()
	if width <= 0 || height <= 0 {
		return
	}
	rows := t.layout(width)
	t.scroll.Layout(len(rows), height)
	first := t.scroll.Offset()
	for y := range height {
		index := first + y
		if index >= len(rows) {
			return
		}
		rows[index].line.Draw(v, 0, y)
	}
}

// layout builds the rows for a width, reusing the last build when nothing changed.
func (t *Transcript) layout(width int) []transcriptRow {
	if t.built && t.width == width {
		return t.rows
	}
	t.rows = t.rows[:0]
	for i, b := range t.blocks {
		if i > 0 {
			// One blank row between blocks. Without it a transcript reads as one wall
			// of text and the eye cannot find where an answer starts.
			t.rows = append(t.rows, transcriptRow{block: i})
		}
		t.appendBlock(i, b, width)
	}
	t.width, t.built = width, true
	return t.rows
}

// appendBlock lays one block out.
func (t *Transcript) appendBlock(index int, b client.Block, width int) {
	switch b.Kind {
	case client.BlockUser:
		t.appendMarked(index, "› ", b.Text, t.Theme.Accent, t.Theme.Text, width)
	case client.BlockAssistant:
		// Split on the text's own newlines first. Wrapping a body that still holds them
		// would drop each one at the cell — a control character has no width — and glue
		// the line before it to the line after with nothing in between.
		t.appendLines(index, linesOf(b.Text, t.Theme.Text), width)
	case client.BlockReasoning:
		t.appendMarked(index, "· ", b.Text, t.Theme.Subtle, t.Theme.Subtle, width)
	case client.BlockNotice:
		t.appendMarked(index, "! ", b.Text, t.Theme.Warning, t.Theme.Muted, width)
	case client.BlockError:
		t.appendMarked(index, "× ", b.Text, t.Theme.Danger, t.Theme.Danger, width)
	case client.BlockTool:
		t.appendTool(index, b, width)
	}
}

// appendMarked lays out a block introduced by a marker, with continuation rows
// indented under it so the block reads as one thing.
func (t *Transcript) appendMarked(index int, marker, body string, markerStyle, bodyStyle grid.Style, width int) {
	indent := text.Width(marker)
	for i, row := range text.WrapAll(linesOf(body, bodyStyle), max(width-indent, 1)) {
		line := row.Line
		if i == 0 {
			line = append(text.Line{{Text: marker, Style: markerStyle}}, line...)
		} else {
			line = append(text.Line{{Text: strings.Repeat(" ", indent)}}, line...)
		}
		t.rows = append(t.rows, transcriptRow{line: line, block: index})
	}
}

// appendLines lays out logical lines, wrapped to the width.
func (t *Transcript) appendLines(index int, lines []text.Line, width int) {
	for _, row := range text.WrapAll(lines, max(width, 1)) {
		t.rows = append(t.rows, transcriptRow{line: row.Line, block: index})
	}
}

// appendTool lays out a tool call: what was run, what it produced, and how it went.
//
// The verdict goes last, under what it is a verdict on. A status line above the
// output would be read as a heading for it, and a call that failed halfway would
// look like it had succeeded.
func (t *Transcript) appendTool(index int, b client.Block, width int) {
	call := b.Tool
	if call == nil {
		return
	}
	head := text.Line{{Text: "● ", Style: t.toolMarker(call.Status)}, {Text: call.Name, Style: t.Theme.Strong}}
	if call.Summary != "" {
		head = append(head,
			text.Span{Text: " · ", Style: t.Theme.Subtle},
			text.Span{Text: text.Truncate(call.Summary, max(width-text.Width(call.Name)-6, 8), "…"), Style: t.Theme.Muted},
		)
	}
	t.rows = append(t.rows, transcriptRow{line: head, block: index})

	if call.Output != "" {
		t.appendOutput(index, call.Output, width)
	}
	if call.Diff != "" {
		t.appendDiff(index, call.Diff, width)
	}
	if call.Status == client.ToolRunning {
		return
	}
	verdict := text.Line{
		{Text: "  " + t.toolMark(call.Status) + " ", Style: t.toolMarker(call.Status)},
	}
	if call.Duration > 0 {
		verdict = append(verdict, text.Span{Text: duration(call.Duration), Style: t.Theme.Muted})
	}
	t.rows = append(t.rows, transcriptRow{line: verdict, block: index})
}

// appendOutput lays out a tool's output in a well, folded to its cap.
func (t *Transcript) appendOutput(index int, output string, width int) {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	shown := len(lines)
	if t.MaxToolRows > 0 {
		shown = min(shown, t.MaxToolRows)
	}
	for _, line := range lines[:shown] {
		t.rows = append(t.rows, transcriptRow{block: index, line: text.Line{
			{Text: "  │ ", Style: t.Theme.Divider},
			{Text: text.Truncate(line, max(width-4, 1), "…"), Style: t.Theme.Muted},
		}})
	}
	if rest := len(lines) - shown; rest > 0 {
		t.rows = append(t.rows, transcriptRow{block: index, line: text.Line{
			{Text: "  │ ", Style: t.Theme.Divider},
			{Text: "… " + more(rest), Style: t.Theme.Subtle},
		}})
	}
}

// appendDiff lays out a unified diff, colouring the two halves.
func (t *Transcript) appendDiff(index int, diff string, width int) {
	for line := range strings.SplitSeq(strings.TrimRight(diff, "\n"), "\n") {
		style := t.Theme.Context
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			style = t.Theme.Subtle
		case strings.HasPrefix(line, "@@"):
			style = t.Theme.Info
		case strings.HasPrefix(line, "+"):
			style = t.Theme.Added
		case strings.HasPrefix(line, "-"):
			style = t.Theme.Removed
		}
		t.rows = append(t.rows, transcriptRow{block: index, line: text.Line{
			{Text: "  "},
			{Text: text.Truncate(line, max(width-2, 1), "…"), Style: style},
		}})
	}
}

func (t *Transcript) toolMarker(status client.ToolStatus) grid.Style {
	switch status {
	case client.ToolOK:
		return t.Theme.Success
	case client.ToolError:
		return t.Theme.Danger
	default:
		return t.Theme.Accent
	}
}

func (t *Transcript) toolMark(status client.ToolStatus) string {
	if status == client.ToolError {
		return "✗"
	}
	return "✓"
}

// more says how much was withheld, in words that read as English either way.
func more(n int) string {
	if n == 1 {
		return "1 more line"
	}
	return strconv.Itoa(n) + " more lines"
}

// linesOf splits a body on newlines.
func linesOf(body string, style grid.Style) []text.Line {
	var out []text.Line
	for line := range strings.SplitSeq(body, "\n") {
		out = append(out, text.Of(line, style))
	}
	return out
}
