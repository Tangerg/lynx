package terminal

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/text"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/promptqueue"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
)

const (
	queueMinWidth = 28
	queueMaxRows  = 3
)

var errQueuedPromptDispatching = errors.New("queued prompt is already being sent")

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
	for index := range visible {
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

func (a *app) enqueueFollowUp(commandID agent.CommandID, message agent.Message) {
	_, err := a.queue.EnqueueCommand(commandID, a.session.ID, message, a.options)
	if err != nil {
		a.message(err.Error())
		return
	}
	a.resetComposer()
	a.operations.Cancel(completionOperation)
	a.completion.Dismiss()
	snapshot := a.syncQueue()
	a.message(fmt.Sprintf("queued follow-up · %d waiting", len(snapshot.Entries)))
}

func (a *app) drainQueue() bool {
	if _, dispatching := a.queue.Dispatching(a.session.ID); dispatching || a.conversation.Busy() || a.following || a.pendingCancel != nil ||
		a.operations.Active(sessionChangeOperation) || a.operations.Active(pendingRunRecoveryOperation) {
		return false
	}
	entry, ok := a.queue.BeginDispatch(a.session.ID)
	if !ok {
		return false
	}
	a.syncQueue()
	if !a.startRun(entry.CommandID, entry.Message, entry.Options, "starting queued follow-up") {
		a.queue.ReleaseDispatch(a.session.ID)
		a.syncQueue()
		return false
	}
	return true
}

func (a *app) commitQueuedDispatch() {
	if _, ok := a.queue.Dispatching(a.session.ID); !ok {
		return
	}
	_, err := a.queue.CommitDispatch(a.session.ID)
	if err != nil {
		a.message(err.Error())
		return
	}
	a.syncQueue()
}

func (a *app) releaseQueuedDispatch() {
	if !a.queue.ReleaseDispatch(a.session.ID) {
		return
	}
	a.syncQueue()
}

func (a *app) syncQueue() promptqueue.Snapshot {
	snapshot := a.queue.Snapshot(a.session.ID)
	a.queueView.Set(snapshot)
	a.prompt.SetQueued(len(snapshot.Entries))
	if a.queueDrawer != nil {
		a.queueDrawer.Set(snapshot)
	}
	if a.queueDialog != nil {
		a.queueDialog.SetTitle("Queue · " + countedNoun(len(snapshot.Entries), "prompt"))
		a.queueDialog.SetDescription(fmt.Sprintf("Manage %s waiting behind the current turn", countedNoun(len(snapshot.Entries), "prompt")))
		if len(snapshot.Entries) == 0 && a.queueDialog.Open() {
			a.queueDialog.Dismiss()
		}
	}
	return snapshot
}

func (a *app) persistQueuedRuns() error {
	if a.workbench == nil {
		return nil
	}
	state := a.queue.State(a.session.ID)
	entries := state.Entries
	persisted := a.workbench.PendingRuns(a.session.ID)
	commands := make([]workbench.PendingRun, 0, len(entries))
	for _, entry := range entries {
		pendingState := workbench.PendingRunQueued
		if entry.ID == state.DispatchingID {
			pending, ok := pendingRunByCommandID(persisted, entry.CommandID)
			if !ok {
				// StartRun was already acknowledged. The UI queue retains the
				// entry until SegmentStarted arrives, but it no longer belongs in
				// the durable authoring outbox.
				continue
			}
			pendingState = pending.State
		}
		commands = append(commands, workbench.PendingRun{
			State: workbench.PendingRunQueued,
			Command: agent.StartRun{
				CommandID: entry.CommandID, SessionID: entry.SessionID,
				Message: entry.Message.Clone(), Options: entry.Options.Clone(),
			},
		})
		commands[len(commands)-1].State = pendingState
	}
	return a.workbench.SavePendingRuns(a.session.ID, commands)
}

// commitQueueMutation keeps the in-memory queue and durable authoring outbox
// on the same side of every edit. Runtime control starts only after this
// transaction succeeds, so a failed disk write cannot silently change FIFO
// order, content, or command identity for the live process alone.
func (a *app) commitQueueMutation(mutate func() error) error {
	before := a.queue.State(a.session.ID)
	if err := mutate(); err != nil {
		return a.rollbackQueueMutation(before, err)
	}
	if err := a.persistQueuedRuns(); err != nil {
		return a.rollbackQueueMutation(before, err)
	}
	a.syncQueue()
	return nil
}

func (a *app) rollbackQueueMutation(before promptqueue.State, cause error) error {
	if err := a.queue.RestoreState(a.session.ID, before); err != nil {
		return errors.Join(cause, fmt.Errorf("restore prompt queue: %w", err))
	}
	a.syncQueue()
	return cause
}

func pendingRunByCommandID(pending []workbench.PendingRun, commandID agent.CommandID) (workbench.PendingRun, bool) {
	for _, candidate := range pending {
		if candidate.Command.CommandID == commandID {
			return candidate, true
		}
	}
	return workbench.PendingRun{}, false
}

func (a *app) ShowQueue() {
	snapshot := a.syncQueue()
	if len(snapshot.Entries) == 0 {
		a.message("queue is empty")
		return
	}
	a.queueDrawer.ResetNotice()
	a.queueDialog.Show()
	a.message(fmt.Sprintf("queue · %d waiting", len(snapshot.Entries)))
}

func (a *app) buildQueueDrawer(theme kit.Theme, glyphs kit.Glyphs, keys *keymap.Map) {
	drawer := newQueueDrawer(theme, glyphs, keys, a.loop.Clipboard())
	dialog := headless.NewDialog(headless.DialogConfig{Stack: &a.stack, Title: "Queue", Content: drawer})
	drawer.SetActions(queueDrawerActions{
		BeginEdit:  a.holdQueuedPrompt,
		SaveEdit:   a.saveQueuedPrompt,
		CancelEdit: a.releaseQueuedPrompt,
		Remove:     a.removeQueuedPrompt,
		Move:       a.moveQueuedPrompt,
		SendNow:    a.sendQueuedNow,
		Dismiss:    dialog.Dismiss,
	})
	a.queueDrawer = drawer
	a.queueDialog = dialog
}

func (a *app) holdQueuedPrompt(entry promptqueue.Entry) error {
	if entry.SessionID != a.session.ID {
		return errors.New("queued prompt belongs to another session")
	}
	if dispatching, ok := a.queue.Dispatching(entry.SessionID); ok && dispatching.ID == entry.ID {
		return errQueuedPromptDispatching
	}
	if err := a.queue.Hold(entry.SessionID, entry.ID); err != nil {
		return err
	}
	a.syncQueue()
	return nil
}

func (a *app) saveQueuedPrompt(entry promptqueue.Entry, message agent.Message, sendNow bool) error {
	if entry.SessionID != a.session.ID {
		return errors.New("queued prompt belongs to another session")
	}
	if dispatching, ok := a.queue.Dispatching(entry.SessionID); ok && dispatching.ID == entry.ID {
		return errQueuedPromptDispatching
	}
	if err := a.commitQueueMutation(func() error {
		if err := a.queue.Update(entry.SessionID, entry.ID, message); err != nil {
			return err
		}
		return a.queue.Release(entry.SessionID, entry.ID)
	}); err != nil {
		return err
	}
	if sendNow {
		return a.sendQueuedNow(entry.ID)
	}
	a.drainQueue()
	return nil
}

func (a *app) releaseQueuedPrompt(entry promptqueue.Entry) error {
	if err := a.queue.Release(entry.SessionID, entry.ID); err != nil {
		return err
	}
	if entry.SessionID != a.session.ID {
		return nil
	}
	a.syncQueue()
	a.drainQueue()
	return nil
}

func (a *app) removeQueuedPrompt(id uint64) error {
	if entry, ok := a.queue.Dispatching(a.session.ID); ok && id == entry.ID {
		return errQueuedPromptDispatching
	}
	if err := a.commitQueueMutation(func() error {
		_, err := a.queue.Remove(a.session.ID, id)
		return err
	}); err != nil {
		return err
	}
	snapshot := a.queue.Snapshot(a.session.ID)
	if len(snapshot.Entries) == 0 {
		a.message("queue is empty")
	}
	return nil
}

func (a *app) moveQueuedPrompt(id uint64, offset int) error {
	if entry, ok := a.queue.Dispatching(a.session.ID); ok && id == entry.ID {
		return errQueuedPromptDispatching
	}
	if err := a.commitQueueMutation(func() error {
		return a.queue.Move(a.session.ID, id, offset)
	}); err != nil {
		return err
	}
	return nil
}

// sendQueuedNow persists priority in the queue before touching the active run.
// Cancellation and dispatch therefore remain resumable if either runtime control
// call is delayed or fails: the promoted entry is still the next FIFO item.
func (a *app) sendQueuedNow(id uint64) error {
	if entry, ok := a.queue.Dispatching(a.session.ID); ok && id == entry.ID {
		return errQueuedPromptDispatching
	}
	if a.operations.Active(sessionChangeOperation) {
		return errors.New("wait for the current session change to finish")
	}
	if err := a.commitQueueMutation(func() error {
		return a.queue.Promote(a.session.ID, id)
	}); err != nil {
		return err
	}
	if a.conversation.Busy() || a.following || a.pendingCancel != nil {
		a.cancel()
		return nil
	}
	if !a.drainQueue() {
		return errors.New("queued prompt could not be dispatched")
	}
	return nil
}
