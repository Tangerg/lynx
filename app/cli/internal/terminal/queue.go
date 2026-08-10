package terminal

import (
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/promptqueue"
)

const (
	queueMinWidth = 28
	queueMaxRows  = 3
)

type queueView struct {
	theme    kit.Theme
	glyphs   kit.Glyphs
	snapshot promptqueue.Snapshot
}

func newQueueView(theme kit.Theme, glyphs kit.Glyphs) *queueView {
	return &queueView{theme: theme, glyphs: glyphs}
}

func (q *queueView) Set(snapshot promptqueue.Snapshot) {
	q.snapshot = snapshot
}

func (q *queueView) Measure(width int) int {
	if width < queueMinWidth || len(q.snapshot.Entries) == 0 {
		return 0
	}
	return min(len(q.snapshot.Entries)+1, queueMaxRows)
}

func (q *queueView) Draw(view grid.View) {
	width, height := view.Size()
	if width < queueMinWidth || height <= 0 || len(q.snapshot.Entries) == 0 {
		return
	}
	header := q.glyphs.Expanded + " Queue"
	view.Text(0, 0, header, q.theme.Heading)
	count := fmt.Sprintf("%d queued", len(q.snapshot.Entries))
	if text.Width(header)+text.Width(count)+2 <= width {
		view.Text(width-text.Width(count), 0, count, q.theme.Subtle)
	}

	capacity := height - 1
	visible := min(capacity, len(q.snapshot.Entries))
	if len(q.snapshot.Entries) > capacity && capacity > 1 {
		visible--
	}
	for index := 0; index < visible; index++ {
		entry := q.snapshot.Entries[index]
		style, marker := q.theme.Muted, q.glyphs.Free
		if index == 0 {
			style, marker = q.theme.Accent, q.glyphs.Marker
		}
		view.Text(0, index+1, q.glyphs.Vertical, q.theme.Divider)
		label := fmt.Sprintf("%s %d. %s", marker, index+1, queueEntryLabel(entry))
		view.Text(2, index+1, text.Truncate(label, max(width-2, 1), q.glyphs.Ellipsis), style)
	}
	if visible < capacity && visible < len(q.snapshot.Entries) {
		remaining := len(q.snapshot.Entries) - visible
		row := visible + 1
		view.Text(0, row, q.glyphs.Vertical, q.theme.Divider)
		view.Text(2, row, fmt.Sprintf("%s %d more", q.glyphs.Ellipsis, remaining), q.theme.Subtle)
	}
}

func queueEntryLabel(entry promptqueue.Entry) string {
	label := strings.TrimSpace(entry.Message.Text)
	if line, _, ok := strings.Cut(label, "\n"); ok {
		label = strings.TrimSpace(line)
	}
	attachments := len(entry.Message.Attachments)
	if label == "" && attachments > 0 {
		label = "@" + entry.Message.Attachments[0].Name
		attachments--
	}
	if attachments > 0 {
		label += " · " + countedNoun(attachments, "attachment")
	}
	return label
}

func countedNoun(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
}

func (a *app) enqueueFollowUp(message client.Message) {
	if _, err := a.queue.Enqueue(a.session.ID, message); err != nil {
		a.message(err.Error())
		return
	}
	a.history.Add(message)
	a.resetComposer()
	a.operations.Cancel(completionOperation)
	a.completion.Dismiss()
	snapshot := a.syncQueue()
	a.message(fmt.Sprintf("queued follow-up · %d waiting", len(snapshot.Entries)))
}

func (a *app) drainQueue() bool {
	if a.dispatchingQueueEntry != 0 || a.state.Busy() || a.following || a.pendingCancel != nil ||
		a.operations.Active(sessionChangeOperation) {
		return false
	}
	entry, ok := a.queue.Next(a.session.ID)
	if !ok {
		return false
	}
	a.dispatchingQueueEntry = entry.ID
	a.syncQueue()
	if !a.startRun(entry.Message, "starting queued follow-up") {
		a.dispatchingQueueEntry = 0
		a.syncQueue()
		return false
	}
	return true
}

func (a *app) commitQueuedDispatch() {
	if a.dispatchingQueueEntry == 0 {
		return
	}
	_, _ = a.queue.Remove(a.session.ID, a.dispatchingQueueEntry)
	a.dispatchingQueueEntry = 0
	a.syncQueue()
}

func (a *app) discardQueuedDispatch() {
	if a.dispatchingQueueEntry == 0 {
		return
	}
	_, _ = a.queue.Remove(a.session.ID, a.dispatchingQueueEntry)
	a.dispatchingQueueEntry = 0
	a.syncQueue()
}

func (a *app) releaseQueuedDispatch() {
	if a.dispatchingQueueEntry == 0 {
		return
	}
	a.dispatchingQueueEntry = 0
	a.syncQueue()
}

func (a *app) syncQueue() promptqueue.Snapshot {
	snapshot := a.queue.Snapshot(a.session.ID)
	a.queueView.Set(snapshot)
	return snapshot
}

func (a *app) ShowQueue() {
	snapshot := a.syncQueue()
	if len(snapshot.Entries) == 0 {
		a.message("queue is empty")
		return
	}
	lines := make([]string, 0, len(snapshot.Entries))
	for index, entry := range snapshot.Entries {
		lines = append(lines, fmt.Sprintf("%d. %s", index+1, queueEntryLabel(entry)))
	}
	a.transcript.Append(&kit.Message{
		Theme: a.transcript.theme, Speaker: "queue", Body: strings.Join(lines, "\n"),
	})
	a.transcript.Retain(a.loop)
	a.message(fmt.Sprintf("queue · %d waiting", len(snapshot.Entries)))
}
