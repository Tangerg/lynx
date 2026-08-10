package terminal

import (
	"errors"
	"image"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/promptqueue"
)

const queueManagerVisibleRows = 3

type queueTargetKind uint8

const (
	queueTargetNone queueTargetKind = iota
	queueTargetRow
	queueTargetSend
	queueTargetEdit
	queueTargetRemove
)

type queueTarget struct {
	kind queueTargetKind
	id   uint64
}

type queueHit struct {
	area   image.Rectangle
	target queueTarget
}

type queuePresentation struct {
	hits       []queueHit
	rowRows    int
	editorArea image.Rectangle
}

type queueManagerActions struct {
	BeginEdit  func(uint64) error
	SaveEdit   func(uint64, client.Message, bool) error
	CancelEdit func(uint64) error
	Remove     func(uint64) error
	Move       func(uint64, int) error
	SendNow    func(uint64) error
	Dismiss    func()
}

// queueManager owns only the transient state of the bottom queue drawer. Queue
// entries and ordering remain in the application store, and every mutation is
// delegated to the app so runtime lifecycle rules stay outside the terminal view.
type queueManager struct {
	theme   kit.Theme
	glyphs  kit.Glyphs
	keys    *keymap.Map
	actions queueManagerActions

	snapshot   promptqueue.Snapshot
	selected   int
	selectedID uint64
	viewport   int

	editingID      uint64
	editingMessage client.Message
	editor         kit.Composer
	focused        bool

	hovered queueTarget
	pressed queueTarget
	dragged bool
	notice  string

	presentation headless.Snapshot[queuePresentation]
	editorRegion headless.PointerRegion
}

func newQueueManager(theme kit.Theme, glyphs kit.Glyphs, keys *keymap.Map, clipboard headless.Clipboard) *queueManager {
	manager := &queueManager{theme: theme, glyphs: glyphs, keys: keys}
	manager.editor = kit.Composer{Theme: theme, Prompt: glyphs.Marker + " ", MaxRows: 4}
	manager.editor.Editor().Keys = keys
	manager.editor.Editor().Clipboard = clipboard
	manager.editor.Editor().Placeholder = "Edit queued prompt"
	return manager
}

func (q *queueManager) SetActions(actions queueManagerActions) { q.actions = actions }

func (q *queueManager) Set(snapshot promptqueue.Snapshot) {
	q.snapshot = snapshot
	entries := snapshot.Entries
	if len(entries) == 0 {
		q.selected, q.selectedID, q.viewport = 0, 0, 0
		q.cancelEditState()
		return
	}
	if q.selectedID != 0 {
		if index := queueEntryIndex(entries, q.selectedID); index >= 0 {
			q.selected = index
		} else {
			q.selected = min(q.selected, len(entries)-1)
		}
	} else {
		q.selected = min(q.selected, len(entries)-1)
	}
	q.selectedID = entries[q.selected].ID
	if q.editingID != 0 && queueEntryIndex(entries, q.editingID) < 0 {
		q.cancelEditState()
	}
	q.ensureVisible()
}

func (q *queueManager) ResetNotice() { q.notice = "" }

func (q *queueManager) Editing() bool { return q.editingID != 0 }

func (q *queueManager) Handle(event input.Event) bool {
	if key, ok := event.(input.Key); ok && key.Down() {
		return q.handleKey(key)
	}
	if mouse, ok := event.(input.Mouse); ok {
		return q.handleMouse(mouse)
	}
	if q.Editing() {
		return q.editor.Handle(event)
	}
	return false
}

func (q *queueManager) handleKey(key input.Key) bool {
	if q.Editing() {
		return q.handleEditKey(key)
	}
	if key.Is(input.Enter, input.Ctrl) {
		q.sendSelected()
		return true
	}
	switch key.Code {
	case input.Up:
		q.moveSelection(-1)
		return true
	case input.Down:
		q.moveSelection(1)
		return true
	case input.Home:
		q.selectIndex(0)
		return true
	case input.End:
		q.selectIndex(len(q.snapshot.Entries) - 1)
		return true
	case input.Delete, input.Backspace:
		q.removeSelected()
		return true
	case input.Enter:
		q.beginEdit()
		return true
	case input.Character:
		if key.Mods != 0 && key.Mods != input.Shift {
			return false
		}
		switch key.Rune {
		case 'q':
			q.dismiss()
		case 'j':
			q.moveSelection(1)
		case 'k':
			q.moveSelection(-1)
		case 'J':
			q.moveSelected(1)
		case 'K':
			q.moveSelected(-1)
		case 'e':
			q.beginEdit()
		case 'x':
			q.removeSelected()
		case 's':
			q.sendSelected()
		default:
			return false
		}
		return true
	default:
		return false
	}
}

func (q *queueManager) handleEditKey(key input.Key) bool {
	if key.Code == input.Esc {
		q.cancelEdit()
		return true
	}
	action, _ := q.keys.Action(key.Chord())
	if action == cancelRun && q.editor.Empty() {
		q.cancelEdit()
		return true
	}
	if key.Code == input.Enter {
		switch {
		case key.Mods == input.Ctrl:
			q.saveEdit(true)
		case action == insertNewline || key.Mods == input.Shift || key.Mods == input.Alt:
			q.editor.Editor().Insert("\n")
		default:
			q.saveEdit(false)
		}
		return true
	}
	return q.editor.Handle(key)
}

func (q *queueManager) handleMouse(mouse input.Mouse) bool {
	if q.Editing() {
		handled, delivered := q.editorRegion.Handle(mouse)
		return handled || delivered
	}
	switch mouse.Action {
	case input.WheelUp:
		q.moveSelection(-1)
		return true
	case input.WheelDown:
		q.moveSelection(1)
		return true
	}
	target := q.hitAt(mouse.Pos)
	switch mouse.Action {
	case input.MouseMove:
		q.hovered = target
	case input.MouseDown:
		if mouse.Button != input.ButtonLeft {
			return false
		}
		q.hovered, q.pressed, q.dragged = target, target, false
	case input.MouseDrag:
		q.hovered = target
		if target != q.pressed {
			q.dragged = true
		}
	case input.MouseUp:
		if mouse.Button != input.ButtonLeft {
			return false
		}
		commit := target.kind != queueTargetNone && target == q.pressed && !q.dragged
		q.hovered, q.pressed, q.dragged = target, queueTarget{}, false
		if commit {
			q.activate(target)
		}
	}
	return true
}

func (q *queueManager) Focus(has bool) {
	q.focused = has
	q.editor.Focus(has && q.Editing())
}

func (q *queueManager) Closed() {
	q.focused = false
	if q.editingID != 0 && q.actions.CancelEdit != nil {
		_ = q.actions.CancelEdit(q.editingID)
	}
	q.cancelEditState()
	q.hovered, q.pressed, q.dragged = queueTarget{}, queueTarget{}, false
}

func (q *queueManager) selectedEntry() (promptqueue.Entry, bool) {
	if q.selected < 0 || q.selected >= len(q.snapshot.Entries) {
		return promptqueue.Entry{}, false
	}
	return q.snapshot.Entries[q.selected], true
}

func (q *queueManager) entry(id uint64) (promptqueue.Entry, bool) {
	index := queueEntryIndex(q.snapshot.Entries, id)
	if index < 0 {
		return promptqueue.Entry{}, false
	}
	return q.snapshot.Entries[index], true
}

func (q *queueManager) selectIndex(index int) {
	if len(q.snapshot.Entries) == 0 {
		return
	}
	q.selected = min(max(index, 0), len(q.snapshot.Entries)-1)
	q.selectedID = q.snapshot.Entries[q.selected].ID
	q.ensureVisible()
	q.hovered = queueTarget{}
}

func (q *queueManager) moveSelection(delta int) { q.selectIndex(q.selected + delta) }

func (q *queueManager) ensureVisible() {
	rows := max(q.presentation.Value().rowRows, 1)
	q.viewport = visibleQueueStart(q.viewport, q.selected, rows, len(q.snapshot.Entries))
}

func (q *queueManager) beginEdit() {
	entry, ok := q.selectedEntry()
	if !ok {
		return
	}
	if q.actions.BeginEdit != nil {
		if err := q.actions.BeginEdit(entry.ID); err != nil {
			q.notice = err.Error()
			return
		}
	}
	q.editingID = entry.ID
	q.editingMessage = cloneMessage(entry.Message)
	q.editor.SetText(entry.Message.Text)
	q.editor.Focus(q.focused)
	q.notice = ""
}

func (q *queueManager) cancelEdit() {
	if q.editingID != 0 && q.actions.CancelEdit != nil {
		if err := q.actions.CancelEdit(q.editingID); err != nil {
			q.notice = err.Error()
			return
		}
	}
	q.cancelEditState()
	q.notice = "edit discarded"
}

func (q *queueManager) cancelEditState() {
	q.editingID = 0
	q.editingMessage = client.Message{}
	q.editor.Reset()
	q.editor.Focus(false)
}

func (q *queueManager) saveEdit(sendNow bool) {
	if q.editingID == 0 || q.actions.SaveEdit == nil {
		return
	}
	id := q.editingID
	message := cloneMessage(q.editingMessage)
	message.Text = strings.TrimSpace(q.editor.Text())
	if err := q.actions.SaveEdit(id, message, sendNow); err != nil {
		q.notice = err.Error()
		return
	}
	q.cancelEditState()
	q.notice = "queued prompt updated"
	if sendNow {
		q.notice = "queued prompt promoted for immediate send"
	}
}

func (q *queueManager) removeSelected() {
	entry, ok := q.selectedEntry()
	if !ok || q.actions.Remove == nil {
		return
	}
	if err := q.actions.Remove(entry.ID); err != nil {
		q.notice = err.Error()
		return
	}
	q.notice = "queued prompt removed"
}

func (q *queueManager) moveSelected(offset int) {
	entry, ok := q.selectedEntry()
	if !ok || q.actions.Move == nil {
		return
	}
	if err := q.actions.Move(entry.ID, offset); err != nil {
		if !errors.Is(err, promptqueue.ErrMoveUnavailable) {
			q.notice = err.Error()
		}
		return
	}
	q.notice = "queued prompt reordered"
}

func (q *queueManager) sendSelected() {
	entry, ok := q.selectedEntry()
	if ok {
		q.send(entry.ID)
	}
}

func (q *queueManager) send(id uint64) {
	if q.actions.SendNow == nil {
		return
	}
	if err := q.actions.SendNow(id); err != nil {
		q.notice = err.Error()
		return
	}
	q.notice = "queued prompt promoted for immediate send"
}

func (q *queueManager) dismiss() {
	if q.actions.Dismiss != nil {
		q.actions.Dismiss()
	}
}

func (q *queueManager) activate(target queueTarget) {
	q.selectID(target.id)
	switch target.kind {
	case queueTargetRow:
	case queueTargetSend:
		q.sendSelected()
	case queueTargetEdit:
		q.beginEdit()
	case queueTargetRemove:
		q.removeSelected()
	}
}

func (q *queueManager) selectID(id uint64) {
	if index := queueEntryIndex(q.snapshot.Entries, id); index >= 0 {
		q.selectIndex(index)
	}
}

func (q *queueManager) hitAt(point image.Point) queueTarget {
	hits := q.presentation.Value().hits
	for _, hit := range slices.Backward(hits) {
		if point.In(hit.area) {
			return hit.target
		}
	}
	return queueTarget{}
}

func queueEntryIndex(entries []promptqueue.Entry, id uint64) int {
	return slices.IndexFunc(entries, func(entry promptqueue.Entry) bool { return entry.ID == id })
}

func visibleQueueStart(current, selected, rows, entries int) int {
	rows = max(rows, 1)
	if selected < current {
		current = selected
	}
	if selected >= current+rows {
		current = selected + 1 - rows
	}
	return min(max(current, 0), max(entries-rows, 0))
}
