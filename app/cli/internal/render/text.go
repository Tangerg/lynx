// Package render writes a run's events out for a reader — a person at a
// terminal, or a program on the other end of a pipe.
//
// Both renderers here are write-only and stateful in the same narrow way: they
// remember which block is streaming so a delta can be placed. Neither keeps a
// transcript. Holding the whole conversation in memory is the TUI's job.
package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

// maxToolOutputLines caps how much of a tool's output is shown. A test run that
// prints ten thousand lines should not bury the answer that follows it.
const maxToolOutputLines = 12

// Text renders events as plain text, with no color and no cursor movement, so
// the same bytes are correct on a terminal, in a pipe and in a log file.
//
// Assistant prose is written as it streams, because that is the content a reader
// is waiting for. Everything else is held until its block completes and then
// printed as a labeled unit — a marker prefix cannot survive being interleaved
// with a live token feed.
type Text struct {
	w   io.Writer
	err error

	// streaming is the id of the assistant block currently being written
	// straight through, empty when none is.
	streaming string
	// pending collects the bodies of blocks that print on completion.
	pending map[string]*strings.Builder
	// column tracks whether the cursor sits mid-line, so separators can be
	// inserted without doubling blank lines.
	column bool
}

// NewText builds a plain-text renderer over w.
func NewText(w io.Writer) *Text {
	return &Text{w: w, pending: make(map[string]*strings.Builder)}
}

// Render writes one event. The first error is remembered and returned by every
// later call, so a caller may render a whole run and check once.
func (t *Text) Render(envelope client.Envelope) error {
	if t.err != nil {
		return t.err
	}
	if err := client.ValidateEvent(envelope.Event); err != nil {
		t.err = fmt.Errorf("render text event: %w", err)
		return t.err
	}
	t.renderEvent(envelope.Event)
	return t.err
}

func (t *Text) renderEvent(event client.Event) {
	switch event := event.(type) {
	case client.RunStarted:
		// A run's identity is machinery, not content.
	case client.BlockStarted:
		t.begin(event.Block)
	case client.BlockDelta:
		t.delta(event)
	case client.BlockCompleted:
		t.finish(event.Block)
	case client.PlanChanged:
		t.plan(event.Items)
	case client.RunResumed:
		// Control identity is not reader-facing text.
	case client.RunInterrupted:
		t.interrupted(event.Interaction)
	case client.RunFinished:
		t.finished(event)
	default:
		t.err = fmt.Errorf("render text event: unsupported event %T", event)
	}
}

// Close ends the output on its own line.
func (t *Text) Close() error {
	t.endLine()
	return t.err
}

func (t *Text) begin(b client.Block) {
	switch b.Kind {
	case client.BlockAssistant:
		t.blank()
		t.streaming = b.ID
		t.write(b.Text)
	case client.BlockReasoning, client.BlockTool, client.BlockUser, client.BlockNotice, client.BlockError:
		body := &strings.Builder{}
		body.WriteString(b.Text)
		t.pending[b.ID] = body
	}
}

func (t *Text) delta(d client.BlockDelta) {
	if d.BlockID == t.streaming {
		t.write(d.Text)
		return
	}
	if body, ok := t.pending[d.BlockID]; ok {
		body.WriteString(d.Text)
	}
}

func (t *Text) finish(b client.Block) {
	if b.ID == t.streaming {
		t.streaming = ""
		t.endLine()
		delete(t.pending, b.ID)
		return
	}
	text := t.completedText(b)
	delete(t.pending, b.ID)
	t.renderCompletedBlock(b, text)
}

func (t *Text) completedText(block client.Block) string {
	if block.Text != "" {
		return block.Text
	}
	if body, ok := t.pending[block.ID]; ok {
		return body.String()
	}
	return ""
}

func (t *Text) renderCompletedBlock(b client.Block, text string) {
	switch b.Kind {
	case client.BlockUser:
		t.userBlock(b, text)
	case client.BlockAssistant:
		t.proseBlock(text)
	case client.BlockReasoning:
		t.blank()
		t.block("· ", text)
	case client.BlockTool:
		t.tool(b)
	case client.BlockNotice:
		t.blank()
		t.block("! ", text)
	case client.BlockError:
		t.blank()
		t.block("× ", text)
	}
}

func (t *Text) userBlock(block client.Block, text string) {
	t.blank()
	if text != "" {
		t.block("› ", text)
	}
	for _, attachment := range block.Attachments {
		t.line("  @ " + attachment.Name + " (" + attachment.MimeType + ", " + strconv.FormatInt(attachment.Size, 10) + " bytes)")
	}
}

func (t *Text) proseBlock(text string) {
	t.blank()
	t.write(text)
	t.endLine()
}

func (t *Text) tool(b client.Block) {
	call := b.Tool
	if call == nil {
		return
	}
	// A running tool announces itself only once its result is in: printing a
	// header, then a body arriving later, reads as two events rather than one.
	if call.Status == client.ToolRunning {
		return
	}
	t.blank()
	t.toolHeader(call)
	t.toolBody(call)
	t.toolVerdict(call)
}

func (t *Text) toolHeader(call *client.ToolCall) {
	head := "● " + textToolName(call)
	primary := textToolPrimary(call)
	if primary != "" {
		head += " · " + primary
	}
	if call.Summary != "" && call.Summary != primary {
		head += " · " + call.Summary
	}
	t.line(head)
}

func (t *Text) toolBody(call *client.ToolCall) {
	if call.Output != "" {
		lines := strings.Split(strings.TrimRight(call.Output, "\n"), "\n")
		shown := min(len(lines), maxToolOutputLines)
		for _, l := range lines[:shown] {
			t.line("  │ " + l)
		}
		if rest := len(lines) - shown; rest > 0 {
			t.line("  │ … " + strconv.Itoa(rest) + " more lines")
		}
	}
	if call.Diff != "" {
		t.diff(call.Diff)
	}
}

func (t *Text) toolVerdict(call *client.ToolCall) {
	// The verdict goes last, under what it is a verdict on.
	mark := "✓"
	if call.Status == client.ToolError {
		mark = "✗"
	}
	status := "  " + mark
	if call.ExitCode != nil && *call.ExitCode != 0 {
		status += " exit " + strconv.Itoa(*call.ExitCode)
	}
	if call.Duration > 0 {
		status += " " + duration(call.Duration)
	}
	t.line(status)
}

func textToolName(call *client.ToolCall) string {
	switch call.Kind {
	case client.ToolShell:
		return "shell"
	case client.ToolEdit:
		return "edit"
	case client.ToolRead:
		return "read"
	case client.ToolSearch:
		return "search"
	case client.ToolWeb:
		return "web"
	case client.ToolTask:
		return "task"
	case client.ToolUnknown:
		if call.Name != "" {
			return call.Name
		}
		return "tool"
	default:
		return "tool"
	}
}

func textToolPrimary(call *client.ToolCall) string {
	var value string
	switch call.Kind {
	case client.ToolShell:
		value = call.Command
	case client.ToolEdit, client.ToolRead:
		value = call.Path
	case client.ToolSearch:
		value = call.Query
	case client.ToolWeb:
		value = call.URL
	case client.ToolUnknown, client.ToolTask:
		// These kinds have no more specific primary field.
	default:
	}
	if value != "" {
		return value
	}
	return call.Summary
}

func (t *Text) diff(d string) {
	for l := range strings.SplitSeq(strings.TrimRight(d, "\n"), "\n") {
		t.line("  " + l)
	}
}

func (t *Text) plan(items []client.PlanItem) {
	if len(items) == 0 {
		return
	}
	t.blank()
	t.line("plan")
	for _, it := range items {
		mark := "☐"
		switch it.Status {
		case client.PlanActive:
			mark = "▸"
		case client.PlanDone:
			mark = "☑"
		case client.PlanPending:
			// The empty checkbox is already selected.
		default:
		}
		t.line("  " + mark + " " + it.Title)
	}
}

func (t *Text) interrupted(interaction client.Interaction) {
	t.blank()
	switch item := interaction.(type) {
	case client.Approval:
		t.line("? " + item.Title)
		if item.Detail != "" {
			t.block("  ", item.Detail)
		}
		if item.Diff != "" {
			t.diff(item.Diff)
		}
	case client.Question:
		t.line("? " + item.Title)
		for _, field := range item.Fields {
			t.line("  - " + field.Label)
		}
	}
}

func (t *Text) finished(e client.RunFinished) {
	t.blank()
	if e.Outcome.Status != client.OutcomeCompleted {
		msg := string(e.Outcome.Status)
		if e.Outcome.Error != "" {
			msg += ": " + e.Outcome.Error
		}
		t.line(msg)
	}
	u := e.Usage
	parts := []string{"↑ " + thousands(u.InputTokens), "↓ " + thousands(u.OutputTokens)}
	if u.CachedTokens > 0 {
		parts = append(parts, "cached "+thousands(u.CachedTokens))
	}
	if u.CostUSD > 0 {
		parts = append(parts, "$"+strconv.FormatFloat(u.CostUSD, 'f', 4, 64))
	}
	if u.Duration > 0 {
		parts = append(parts, duration(u.Duration))
	}
	t.line(strings.Join(parts, "  "))
}

// block writes a body with prefix on the first line and matching indentation on
// the rest, so a multi-line body stays visually one thing.
//
// The indent counts runes, not bytes: the markers here are multi-byte and
// single-width, so a byte count would over-indent every continuation line by the
// marker's encoded length.
func (t *Text) block(prefix, text string) {
	indent := strings.Repeat(" ", utf8.RuneCountInString(prefix))
	first := true
	for l := range strings.SplitSeq(strings.TrimRight(text, "\n"), "\n") {
		if first {
			first = false
			t.line(prefix + l)
			continue
		}
		t.line(indent + l)
	}
}

func (t *Text) line(s string) {
	t.write(s)
	t.endLine()
}

// blank opens a visual gap before a new unit, without stacking gaps.
func (t *Text) blank() {
	t.endLine()
	t.write("\n")
	t.column = false
}

// endLine closes the current line if one is open.
func (t *Text) endLine() {
	if !t.column {
		return
	}
	t.write("\n")
	t.column = false
}

func (t *Text) write(s string) {
	if t.err != nil || s == "" {
		return
	}
	if _, err := io.WriteString(t.w, s); err != nil {
		t.err = err
		return
	}
	t.column = !strings.HasSuffix(s, "\n")
}

// duration prints a span the way a person reads one: sub-second in
// milliseconds, otherwise seconds with one decimal.
func duration(d time.Duration) string {
	if d < time.Second {
		return strconv.FormatInt(d.Milliseconds(), 10) + "ms"
	}
	return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + "s"
}

// thousands groups an integer for reading.
func thousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	first := 0
	if s[0] == '-' {
		first = 1
	}
	var b strings.Builder
	b.Grow(len(s) + (len(s)-first-1)/3)
	b.WriteString(s[:first])
	for i := first; i < len(s); i++ {
		if i > first && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
