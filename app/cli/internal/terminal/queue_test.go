package terminal

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/promptqueue"
	"github.com/Tangerg/lynx/app/cli/internal/settings"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
)

type finishObservingRuntime struct {
	*recordingRuntime
	finished chan struct{}
	once     sync.Once
}

type blockingFirstStartRuntime struct {
	agent.Runtime

	mu             sync.Mutex
	blocked        bool
	receiptCommand agent.CommandID
	receipt        agent.SegmentStream
	receiptErr     error
	inputs         []agent.StartRun
	started        chan agent.StartRun
	release        chan struct{}
}

func (r *blockingFirstStartRuntime) StartRun(ctx context.Context, request agent.StartRun) (agent.SegmentStream, error) {
	r.mu.Lock()
	if request.CommandID != "" && request.CommandID == r.receiptCommand {
		r.inputs = append(r.inputs, request.Clone())
		stream, err := r.receipt, r.receiptErr
		r.mu.Unlock()
		if err != nil {
			return agent.SegmentStream{}, err
		}
		return stream, nil
	}
	first := !r.blocked
	r.blocked = true
	r.inputs = append(r.inputs, request.Clone())
	r.mu.Unlock()
	stream, err := r.Runtime.StartRun(ctx, request)
	if err != nil {
		receipt, accepted := agent.AcceptedMutationReceipt(err)
		if !accepted {
			return agent.SegmentStream{}, err
		}
		stream = receipt
	}
	if !first {
		return stream, err
	}
	r.mu.Lock()
	r.receiptCommand = request.CommandID
	r.receipt = stream
	r.receiptErr = err
	r.mu.Unlock()
	select {
	case r.started <- request.Clone():
	case <-ctx.Done():
		return agent.SegmentStream{}, context.Cause(ctx)
	}
	select {
	case <-r.release:
		if err != nil {
			return agent.SegmentStream{}, err
		}
		return stream, nil
	case <-ctx.Done():
		return agent.SegmentStream{}, context.Cause(ctx)
	}
}

func (r *blockingFirstStartRuntime) startInputs() []agent.StartRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	inputs := make([]agent.StartRun, len(r.inputs))
	for index, input := range r.inputs {
		inputs[index] = input.Clone()
	}
	return inputs
}

func (r *finishObservingRuntime) StartRun(ctx context.Context, request agent.StartRun) (agent.SegmentStream, error) {
	stream, err := r.recordingRuntime.StartRun(ctx, request)
	if err != nil {
		return agent.SegmentStream{}, err
	}
	original := stream.Events
	stream.Events = func(yield func(agent.RunEvent, error) bool) {
		for event, streamErr := range original {
			continued := yield(event, streamErr)
			if streamErr == nil {
				if _, finished := event.Event.(agent.RunFinished); finished {
					r.once.Do(func() { close(r.finished) })
				}
			}
			if !continued {
				return
			}
		}
	}
	return stream, nil
}

func testQueueDrawer(t *testing.T, messages ...agent.Message) (*queueDrawer, *promptqueue.Queue) {
	t.Helper()
	queue := promptqueue.New()
	for _, message := range messages {
		if _, err := queue.Enqueue("session", message); err != nil {
			t.Fatal(err)
		}
	}
	bindings, err := configuredKeyBindings(settings.Default())
	if err != nil {
		t.Fatal(err)
	}
	drawer := newQueueDrawer(kit.Dark(), kit.Unicode(), bindings.editor, nil)
	sync := func() { drawer.Set(queue.Snapshot("session")) }
	drawer.SetActions(queueDrawerActions{
		BeginEdit: func(entry promptqueue.Entry) error {
			err := queue.Hold(entry.SessionID, entry.ID)
			sync()
			return err
		},
		SaveEdit: func(entry promptqueue.Entry, message agent.Message, sendNow bool) error {
			if err := queue.Update(entry.SessionID, entry.ID, message); err != nil {
				return err
			}
			if err := queue.Release(entry.SessionID, entry.ID); err != nil {
				return err
			}
			if sendNow {
				if err := queue.Promote(entry.SessionID, entry.ID); err != nil {
					return err
				}
			}
			sync()
			return nil
		},
		CancelEdit: func(entry promptqueue.Entry) error {
			return queue.Release(entry.SessionID, entry.ID)
		},
		Remove: func(id uint64) error {
			_, err := queue.Remove("session", id)
			sync()
			return err
		},
		Move: func(id uint64, offset int) error {
			err := queue.Move("session", id, offset)
			sync()
			return err
		},
		SendNow: func(id uint64) error {
			err := queue.Promote("session", id)
			sync()
			return err
		},
	})
	sync()
	return drawer, queue
}

func drawQueueDrawer(t *testing.T, drawer *queueDrawer, width, height int) (*headless.Root, *grid.Surface, string) {
	t.Helper()
	root := headless.NewRoot(drawer)
	surface := grid.NewSurface(width, height)
	root.Draw(surface.View())
	return root, surface, strings.Join(surface.Rows(), "\n")
}

func queueDrawerHit(t *testing.T, drawer *queueDrawer, target queueTarget) queueHit {
	t.Helper()
	for _, hit := range drawer.presentation.Value().hits {
		if hit.target == target {
			return hit
		}
	}
	t.Fatalf("queue target %+v is not presented", target)
	return queueHit{}
}

func TestQueueViewKeepsTheNextPromptAndOverflowVisible(t *testing.T) {
	view := newQueueView(kit.Dark(), kit.Unicode())
	view.Set(promptqueue.Snapshot{Entries: []promptqueue.Entry{
		{ID: 1, Message: agent.Message{Text: "first follow-up\nwith more detail"}},
		{ID: 2, Message: agent.Message{Text: "second follow-up"}},
		{ID: 3, Message: agent.Message{Text: "third follow-up"}},
		{ID: 4, Message: agent.Message{Text: "fourth follow-up"}},
	}})
	if got := view.Measure(queueMinWidth - 1); got != 0 {
		t.Fatalf("narrow queue height = %d", got)
	}
	if got := view.Measure(80); got != queueMaxRows {
		t.Fatalf("queue height = %d", got)
	}
	rendered := drawStatic(t, view, 48, queueMaxRows)
	for _, want := range []string{"Queue", "4 queued", "first follow-up", "3 more"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("queue does not contain %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "with more detail") {
		t.Fatalf("queue preview leaked later lines:\n%s", rendered)
	}
}

func TestQueueDrawerRendersPreviewActionsAndResponsiveFallback(t *testing.T) {
	drawer, _ := testQueueDrawer(t,
		agent.Message{Text: "first line\nsecond line", Attachments: []agent.Attachment{{ID: "a", Kind: agent.AttachmentText, Name: "context.txt", Path: "/tmp/context.txt"}}},
		agent.Message{Text: "second prompt"},
	)
	_, _, rendered := drawQueueDrawer(t, drawer, 96, 9)
	for _, want := range []string{
		"Queue · 2 prompts", "Preview · full queued prompt", "second line",
		"[send now]", "[edit]", "[remove]", "J/K reorder",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("queue drawer does not contain %q:\n%s", want, rendered)
		}
	}
	_, _, narrow := drawQueueDrawer(t, drawer, 22, 5)
	if !strings.Contains(narrow, "first line") || strings.Contains(narrow, "[send now]") {
		t.Fatalf("narrow queue drawer did not preserve content priority:\n%s", narrow)
	}
}

func TestQueueDrawerRejectsCommandsAgainstAReplacedPresentation(t *testing.T) {
	bindings, err := configuredKeyBindings(settings.Default())
	if err != nil {
		t.Fatal(err)
	}
	drawer := newQueueDrawer(kit.Dark(), kit.Unicode(), bindings.editor, nil)
	active := true
	drawer.lifecycle.bind(func() bool { return active })
	removed := make([]uint64, 0, 1)
	drawer.SetActions(queueDrawerActions{Remove: func(id uint64) error {
		removed = append(removed, id)
		return nil
	}})
	drawer.Set(promptqueue.Snapshot{Entries: []promptqueue.Entry{{ID: 1, Message: agent.Message{Text: "visible"}}}})
	root := headless.NewRoot(drawer)
	surface := grid.NewSurface(72, 8)
	root.Draw(surface.View())

	drawer.Set(promptqueue.Snapshot{Entries: []promptqueue.Entry{{ID: 2, Message: agent.Message{Text: "replacement"}}}})
	if !root.Handle(input.Key{Code: input.Delete}) || len(removed) != 0 {
		t.Fatalf("undrawn replacement removed entries %v", removed)
	}
	root.Draw(surface.View())
	if !root.Handle(input.Key{Code: input.Delete}) || len(removed) != 1 || removed[0] != 2 {
		t.Fatalf("visible replacement removed entries %v", removed)
	}

	active = false
	root.Handle(input.Key{Code: input.Delete})
	if len(removed) != 1 {
		t.Fatalf("closed drawer removed entries %v", removed)
	}
}

func TestQueueDrawerEditsMultilineTextAndKeepsAttachments(t *testing.T) {
	attachment := agent.Attachment{ID: "a", Kind: agent.AttachmentText, Name: "context.txt", Path: "/tmp/context.txt"}
	drawer, queue := testQueueDrawer(t, agent.Message{Text: "original", Attachments: []agent.Attachment{attachment}})
	drawer.Focus(true)
	drawer.Handle(input.Key{Code: input.Enter})
	if !drawer.Editing() {
		t.Fatal("Enter did not begin queue editing")
	}
	if held := queue.Snapshot("session").Entries[0].Held; !held {
		t.Fatal("queue editor did not hold its entry")
	}
	drawer.Handle(input.Key{Code: input.Character, Rune: 'u', Mods: input.Ctrl})
	drawer.Handle(input.Paste{Text: "edited"})
	drawer.Handle(input.Key{Code: input.Enter, Mods: input.Shift})
	drawer.Handle(input.Paste{Text: "second line"})
	drawer.Handle(input.Key{Code: input.Enter})
	if drawer.Editing() {
		t.Fatal("Enter did not save queue editing")
	}
	entry, ok := queue.BeginDispatch("session")
	if !ok || entry.Message.Text != "edited\nsecond line" || len(entry.Message.Attachments) != 1 || entry.Message.Attachments[0].ID != attachment.ID {
		t.Fatalf("saved queued message = %+v, %v", entry.Message, ok)
	}
	queue.ReleaseDispatch("session")

	drawer.Handle(input.Key{Code: input.Enter})
	drawer.Handle(input.Paste{Text: " discarded"})
	if !drawer.Handle(input.Key{Code: input.Esc}) || drawer.Editing() {
		t.Fatal("first Esc did not discard the edit")
	}
	if drawer.Handle(input.Key{Code: input.Esc}) {
		t.Fatal("browse-mode Esc should be left for the dialog controller")
	}
	entries := queue.Snapshot("session").Entries
	if len(entries) != 1 || entries[0].Message.Text != "edited\nsecond line" {
		t.Fatalf("discard changed queued entries to %+v", entries)
	}
}

func TestQueueDrawerPreservesItsEditThroughAnExtremeResize(t *testing.T) {
	drawer, queue := testQueueDrawer(t, agent.Message{Text: "original"})
	drawer.Focus(true)
	drawer.Handle(input.Key{Code: input.Enter})
	drawer.Handle(input.Key{Code: input.Character, Rune: 'u', Mods: input.Ctrl})
	drawer.Handle(input.Paste{Text: "first line"})
	drawer.Handle(input.Key{Code: input.Enter, Mods: input.Shift})
	drawer.Handle(input.Paste{Text: "second line"})

	_, _, tiny := drawQueueDrawer(t, drawer, 1, 1)
	if tiny == "" {
		t.Fatal("extreme queue layout did not retain ownership of its viewport")
	}
	drawer.Handle(input.Paste{Text: " while tiny"})
	_, _, restored := drawQueueDrawer(t, drawer, 72, 8)
	for _, want := range []string{"Editing queued prompt", "first line", "second line while tiny"} {
		if !strings.Contains(restored, want) {
			t.Fatalf("restored queue editor does not contain %q:\n%s", want, restored)
		}
	}

	drawer.Handle(input.Key{Code: input.Enter})
	entry, ok := queue.BeginDispatch("session")
	if !ok || entry.Message.Text != "first line\nsecond line while tiny" {
		t.Fatalf("saved queue edit after resize = %+v, %v", entry.Message, ok)
	}
}

func TestClosingQueueDrawerReleasesItsEditedEntry(t *testing.T) {
	drawer, queue := testQueueDrawer(t, agent.Message{Text: "editable"})
	drawer.Focus(true)
	drawer.Handle(input.Key{Code: input.Enter})
	if !queue.Snapshot("session").Entries[0].Held {
		t.Fatal("test did not hold the edited entry")
	}
	drawer.Closed()
	if drawer.Editing() || queue.Snapshot("session").Entries[0].Held {
		t.Fatalf("closed queue drawer left editing=%v snapshot=%+v", drawer.Editing(), queue.Snapshot("session"))
	}
}

func TestQueueDrawerReleasesTheOriginalSessionWhenSnapshotChanges(t *testing.T) {
	drawer, queue := testQueueDrawer(t, agent.Message{Text: "old session prompt"})
	if _, err := queue.Enqueue("next-session", agent.Message{Text: "next session prompt"}); err != nil {
		t.Fatal(err)
	}
	drawer.Focus(true)
	drawer.Handle(input.Key{Code: input.Enter})
	if !queue.Snapshot("session").Entries[0].Held {
		t.Fatal("test did not hold the original session entry")
	}

	drawer.Set(queue.Snapshot("next-session"))
	if drawer.Editing() {
		t.Fatal("snapshot replacement retained an editor for another session")
	}
	if queue.Snapshot("session").Entries[0].Held {
		t.Fatal("snapshot replacement left the original session entry held")
	}
	if entries := queue.Snapshot("session").Entries; len(entries) != 1 || entries[0].Message.Text != "old session prompt" || entries[0].Held {
		t.Fatalf("released original entry = %+v", entries)
	}
	_, _, rendered := drawQueueDrawer(t, drawer, 72, 7)
	if !strings.Contains(rendered, "next session prompt") || strings.Contains(rendered, "old session prompt") {
		t.Fatalf("replacement snapshot rendered the wrong session:\n%s", rendered)
	}
}

func TestQueueDrawerEditorOwnsPointerPlacement(t *testing.T) {
	drawer, _ := testQueueDrawer(t, agent.Message{Text: "move this cursor"})
	drawer.Focus(true)
	drawer.Handle(input.Key{Code: input.Enter})
	root, _, _ := drawQueueDrawer(t, drawer, 72, 8)
	area := drawer.presentation.Value().editorArea
	if area.Empty() {
		t.Fatal("queue editor did not publish its pointer area")
	}
	_, before := drawer.editor.Editor().Cursor()
	if before == 0 {
		t.Fatal("queue editor cursor did not start at the end")
	}
	if !root.Handle(input.Mouse{Pos: image.Pt(area.Min.X+2, area.Min.Y), Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("queue editor click was not routed")
	}
	line, column := drawer.editor.Editor().Cursor()
	if line != 0 || column != 0 {
		t.Fatalf("queue editor click moved cursor to %d:%d, want 0:0", line, column)
	}
}

func TestQueueDrawerReordersAndPromotesTheSelectedEntry(t *testing.T) {
	drawer, queue := testQueueDrawer(t,
		agent.Message{Text: "first"}, agent.Message{Text: "second"}, agent.Message{Text: "third"},
	)
	drawer.Handle(input.Key{Code: input.Down})
	drawer.Handle(input.Key{Code: input.Character, Rune: 'J', Mods: input.Shift})
	got := queue.Snapshot("session").Entries
	if got[0].Message.Text != "first" || got[1].Message.Text != "third" || got[2].Message.Text != "second" {
		t.Fatalf("reordered queue = %+v", got)
	}
	drawer.Handle(input.Key{Code: input.Character, Rune: 's'})
	got = queue.Snapshot("session").Entries
	if got[0].Message.Text != "second" {
		t.Fatalf("send-now promotion = %+v", got)
	}
}

func TestQueueDrawerMouseActionsCommitOnlyOnAnUndraggedMatchingRelease(t *testing.T) {
	drawer, queue := testQueueDrawer(t, agent.Message{Text: "first"}, agent.Message{Text: "second"})
	root, surface, _ := drawQueueDrawer(t, drawer, 80, 5)
	firstID := queue.Snapshot("session").Entries[0].ID
	remove := queueDrawerHit(t, drawer, queueTarget{kind: queueTargetRemove, id: firstID})
	edit := queueDrawerHit(t, drawer, queueTarget{kind: queueTargetEdit, id: firstID})

	root.Handle(input.Mouse{Pos: remove.area.Min, Action: input.MouseDown, Button: input.ButtonLeft})
	surface.Reset()
	root.Draw(surface.View())
	pressed, ok := surface.CellAt(remove.area.Min.X, remove.area.Min.Y)
	if !ok || !pressed.Style.Attr.Has(grid.Reverse) {
		t.Fatalf("pressed queue action has no pressed visual: %+v, %v", pressed, ok)
	}
	root.Handle(input.Mouse{Pos: edit.area.Min, Action: input.MouseUp, Button: input.ButtonLeft})
	if got := len(queue.Snapshot("session").Entries); got != 2 {
		t.Fatalf("mismatched release removed an entry: %d", got)
	}

	root.Draw(surface.View())
	remove = queueDrawerHit(t, drawer, queueTarget{kind: queueTargetRemove, id: firstID})
	root.Handle(input.Mouse{Pos: remove.area.Min, Action: input.MouseDown, Button: input.ButtonLeft})
	root.Handle(input.Mouse{Pos: image.Pt(0, 0), Action: input.MouseDrag, Button: input.ButtonLeft})
	root.Handle(input.Mouse{Pos: remove.area.Min, Action: input.MouseUp, Button: input.ButtonLeft})
	if got := len(queue.Snapshot("session").Entries); got != 2 {
		t.Fatalf("dragged release removed an entry: %d", got)
	}

	root.Draw(surface.View())
	remove = queueDrawerHit(t, drawer, queueTarget{kind: queueTargetRemove, id: firstID})
	root.Handle(input.Mouse{Pos: remove.area.Min, Action: input.MouseDown, Button: input.ButtonLeft})
	root.Handle(input.Mouse{Pos: remove.area.Min, Action: input.MouseUp, Button: input.ButtonLeft})
	if got := len(queue.Snapshot("session").Entries); got != 1 {
		t.Fatalf("matching release left %d entries", got)
	}
}

func TestQueueDrawerCancelsAStalePointerGesture(t *testing.T) {
	tests := []struct {
		name      string
		interrupt func(*queueDrawer, *headless.Root, *promptqueue.Queue, image.Point)
	}{
		{
			name: "different button release",
			interrupt: func(_ *queueDrawer, root *headless.Root, _ *promptqueue.Queue, point image.Point) {
				root.Handle(input.Mouse{Pos: point, Action: input.MouseUp, Button: input.ButtonRight})
			},
		},
		{
			name: "different button press",
			interrupt: func(_ *queueDrawer, root *headless.Root, _ *promptqueue.Queue, point image.Point) {
				root.Handle(input.Mouse{Pos: point, Action: input.MouseDown, Button: input.ButtonRight})
			},
		},
		{
			name: "focus loss",
			interrupt: func(drawer *queueDrawer, _ *headless.Root, _ *promptqueue.Queue, _ image.Point) {
				drawer.Focus(false)
				drawer.Focus(true)
			},
		},
		{
			name: "snapshot replacement",
			interrupt: func(drawer *queueDrawer, root *headless.Root, queue *promptqueue.Queue, _ image.Point) {
				drawer.Set(queue.Snapshot("session"))
				root.Draw(grid.NewSurface(80, 5).View())
			},
		},
		{
			name: "keyboard navigation",
			interrupt: func(drawer *queueDrawer, _ *headless.Root, _ *promptqueue.Queue, _ image.Point) {
				drawer.Handle(input.Key{Code: input.Down})
			},
		},
		{
			name: "wheel navigation",
			interrupt: func(_ *queueDrawer, root *headless.Root, _ *promptqueue.Queue, point image.Point) {
				root.Handle(input.Mouse{Pos: point, Action: input.WheelDown})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			drawer, queue := testQueueDrawer(t, agent.Message{Text: "first"}, agent.Message{Text: "second"})
			root, _, _ := drawQueueDrawer(t, drawer, 80, 5)
			firstID := queue.Snapshot("session").Entries[0].ID
			remove := queueDrawerHit(t, drawer, queueTarget{kind: queueTargetRemove, id: firstID})

			root.Handle(input.Mouse{Pos: remove.area.Min, Action: input.MouseDown, Button: input.ButtonLeft})
			test.interrupt(drawer, root, queue, remove.area.Min)
			root.Handle(input.Mouse{Pos: remove.area.Min, Action: input.MouseUp, Button: input.ButtonLeft})
			if got := len(queue.Snapshot("session").Entries); got != 2 {
				t.Fatalf("stale pointer gesture left %d entries", got)
			}
		})
	}
}

func TestDurableQueueKeepsTheOpeningCommandAheadOfPriorityEdits(t *testing.T) {
	store, err := workbench.Open("", workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	queue := promptqueue.New()
	commands := []agent.StartRun{
		{CommandID: agent.CommandID("cli_11111111111111111111111111111111"), SessionID: "session", Message: agent.Message{Text: "opening"}},
		{CommandID: agent.CommandID("cli_22222222222222222222222222222222"), SessionID: "session", Message: agent.Message{Text: "send next"}},
	}
	for _, command := range commands {
		if err := store.StagePendingRun(workbench.PendingRun{State: workbench.PendingRunQueued, Command: command}); err != nil {
			t.Fatal(err)
		}
		if _, err := queue.EnqueueCommand(command.CommandID, command.SessionID, command.Message, command.Options); err != nil {
			t.Fatal(err)
		}
	}
	dispatching, ok := queue.BeginDispatch("session")
	if !ok || dispatching.CommandID != commands[0].CommandID {
		t.Fatalf("opening reservation = %+v, %t", dispatching, ok)
	}
	if err := store.MarkPendingRunDispatching("session", dispatching.CommandID); err != nil {
		t.Fatal(err)
	}
	secondID := queue.Snapshot("session").Entries[1].ID
	if err := queue.Promote("session", secondID); err != nil {
		t.Fatal(err)
	}

	application := &app{queue: queue, workbench: store, session: agent.Session{ID: "session"}}
	if err := application.persistQueuedRuns(); err != nil {
		t.Fatal(err)
	}
	pending := store.PendingRuns("session")
	if len(pending) != 2 || pending[0].Command.CommandID != commands[0].CommandID ||
		pending[0].State != workbench.PendingRunDispatching || pending[1].Command.CommandID != commands[1].CommandID ||
		pending[1].State != workbench.PendingRunQueued {
		t.Fatalf("durable queue crossed opening boundary: %+v", pending)
	}

	if err := store.AcknowledgePendingRun("session", commands[0].CommandID); err != nil {
		t.Fatal(err)
	}
	if err := application.persistQueuedRuns(); err != nil {
		t.Fatal(err)
	}
	pending = store.PendingRuns("session")
	if len(pending) != 1 || pending[0].Command.CommandID != commands[1].CommandID || pending[0].State != workbench.PendingRunQueued {
		t.Fatalf("post-acknowledgement queue = %+v", pending)
	}
	if removed, err := queue.CommitDispatch("session"); err != nil || removed.CommandID != commands[0].CommandID {
		t.Fatalf("committed opening command = %+v, %v", removed, err)
	}
}

func TestQueueMutationRollbackPreservesTheDispatchReservation(t *testing.T) {
	directory := t.TempDir()
	store, err := workbench.Open(directory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	queue := promptqueue.New()
	commands := []agent.StartRun{
		{CommandID: agent.CommandID("cli_11111111111111111111111111111111"), SessionID: "session", Message: agent.Message{Text: "opening"}},
		{CommandID: agent.CommandID("cli_22222222222222222222222222222222"), SessionID: "session", Message: agent.Message{Text: "second"}},
		{CommandID: agent.CommandID("cli_33333333333333333333333333333333"), SessionID: "session", Message: agent.Message{Text: "promote me"}},
	}
	for _, command := range commands {
		if err := store.StagePendingRun(workbench.PendingRun{State: workbench.PendingRunQueued, Command: command}); err != nil {
			t.Fatal(err)
		}
		if _, err := queue.EnqueueCommand(command.CommandID, command.SessionID, command.Message, command.Options); err != nil {
			t.Fatal(err)
		}
	}
	dispatching, ok := queue.BeginDispatch("session")
	if !ok {
		t.Fatal("queue did not reserve its first entry")
	}
	if err := store.MarkPendingRunDispatching("session", dispatching.CommandID); err != nil {
		t.Fatal(err)
	}
	before := queue.State("session")

	stateDirectory := filepath.Join(directory, "sessions")
	states, err := os.ReadDir(stateDirectory)
	if err != nil || len(states) != 1 {
		t.Fatalf("session state files = %d, %v", len(states), err)
	}
	statePath := filepath.Join(stateDirectory, states[0].Name())
	backupPath := statePath + ".backup"
	if err := os.Rename(statePath, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(statePath, 0o700); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(statePath, "blocker")
	if err := os.WriteFile(blocker, []byte("block state replacement"), 0o600); err != nil {
		t.Fatal(err)
	}

	queueView := newQueueView(kit.Dark(), kit.Unicode())
	prompt := &promptView{}
	application := &app{
		queue: queue, workbench: store, session: agent.Session{ID: "session"},
		queueView: queueView, prompt: prompt,
	}
	promotedID := before.Entries[2].ID
	if err := application.commitQueueMutation(func() error {
		return queue.Promote("session", promotedID)
	}); err == nil {
		t.Fatal("queue mutation unexpectedly survived a failed durable replacement")
	}
	after := queue.State("session")
	assertQueueStateEqual(t, after, before)
	if got := queueView.snapshot.Entries; len(got) != len(before.Entries) || got[2].ID != promotedID {
		t.Fatalf("rolled-back queue projection = %+v", got)
	}
	if prompt.queued != len(before.Entries) {
		t.Fatalf("rolled-back prompt queue count = %d, want %d", prompt.queued, len(before.Entries))
	}

	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backupPath, statePath); err != nil {
		t.Fatal(err)
	}
	reopened, err := workbench.Open(directory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pending := reopened.PendingRuns("session")
	if len(pending) != len(commands) || pending[0].State != workbench.PendingRunDispatching {
		t.Fatalf("durable queue after failed mutation = %+v", pending)
	}
	for index, command := range commands {
		if pending[index].Command.CommandID != command.CommandID {
			t.Fatalf("durable command %d = %s, want %s", index, pending[index].Command.CommandID, command.CommandID)
		}
	}
}

func TestRestoredPendingRunStateControlsQueueOwnership(t *testing.T) {
	for _, test := range []struct {
		name        string
		state       workbench.PendingRunState
		dispatching bool
	}{
		{name: "queued", state: workbench.PendingRunQueued},
		{name: "dispatching", state: workbench.PendingRunDispatching, dispatching: true},
		{name: "canceling", state: workbench.PendingRunCanceling, dispatching: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			queue := promptqueue.New()
			application := &app{
				queue: queue, session: agent.Session{ID: "session"},
				queueView: newQueueView(kit.Dark(), kit.Unicode()), prompt: &promptView{},
			}
			pending := []workbench.PendingRun{
				{State: test.state, Command: agent.StartRun{
					CommandID: agent.CommandID("cli_11111111111111111111111111111111"),
					SessionID: "session", Message: agent.Message{Text: "first"},
				}},
				{State: workbench.PendingRunQueued, Command: agent.StartRun{
					CommandID: agent.CommandID("cli_22222222222222222222222222222222"),
					SessionID: "session", Message: agent.Message{Text: "second"},
				}},
			}
			if err := application.restorePendingQueue(pending); err != nil {
				t.Fatal(err)
			}
			dispatching, found := queue.Dispatching("session")
			if found != test.dispatching {
				t.Fatalf("dispatch reservation present = %t, want %t", found, test.dispatching)
			}
			if found && dispatching.CommandID != pending[0].Command.CommandID {
				t.Fatalf("dispatch reservation = %+v", dispatching)
			}
			if got := application.queueView.snapshot.Entries; len(got) != len(pending) || application.prompt.queued != len(pending) {
				t.Fatalf("restored queue projection = %+v, prompt count %d", got, application.prompt.queued)
			}
		})
	}
}

func assertQueueStateEqual(t *testing.T, got, want promptqueue.State) {
	t.Helper()
	if got.DispatchingID != want.DispatchingID || len(got.Entries) != len(want.Entries) {
		t.Fatalf("queue state = %+v, want %+v", got, want)
	}
	for index := range want.Entries {
		actual, expected := got.Entries[index], want.Entries[index]
		if actual.ID != expected.ID || actual.CommandID != expected.CommandID || actual.SessionID != expected.SessionID ||
			actual.Held != expected.Held || !actual.Message.Equal(expected.Message) || !actual.Options.Equal(expected.Options) {
			t.Fatalf("queue entry %d = %+v, want %+v", index, actual, expected)
		}
	}
}

func TestRunningTurnQueuesFollowUpsAndDrainsThemInFIFOOrder(t *testing.T) {
	base := mock.New()
	base.Script = func(prompt string) mock.Script {
		if prompt == "PRIMARY_RUN" {
			return mock.Script{Prelude: []mock.Step{
				{Delay: 500 * time.Millisecond, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
			}}
		}
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockCompleted{Block: agent.Block{ID: "answer-" + prompt, Kind: agent.BlockAssistant, Text: "RAN_" + prompt}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("PRIMARY_RUN")
	host.Press(input.Enter)
	host.Shows(t, "working")

	host.Type("FOLLOW_UP_ONE")
	host.Press(input.Enter)
	host.Type("FOLLOW_UP_TWO")
	host.Press(input.Enter)
	host.Shows(t, "2 queued")
	host.Shows(t, "FOLLOW_UP_ONE")
	host.Shows(t, "RAN_FOLLOW_UP_TWO")
	host.Shows(t, "complete")

	inputs := backend.startInputs()
	if len(inputs) != 3 {
		t.Fatalf("started %d runs: %+v", len(inputs), inputs)
	}
	for index, want := range []string{"PRIMARY_RUN", "FOLLOW_UP_ONE", "FOLLOW_UP_TWO"} {
		if got := inputs[index].Message.Text; got != want {
			t.Fatalf("run %d = %q, want %q", index+1, got, want)
		}
	}
	stop()
}

func TestAcceptedStartRetainsTheFIFOBoundaryUntilDurableSettlementRecovers(t *testing.T) {
	base := mock.New()
	base.Script = func(prompt string) mock.Script {
		finishDelay := time.Duration(0)
		if prompt == "FIRST_SETTLEMENT" {
			finishDelay = time.Second
		}
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockCompleted{Block: agent.Block{
				ID: "answer-" + prompt, Kind: agent.BlockAssistant, Text: prompt + "_RAN",
			}}},
			{Delay: finishDelay, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	gate := &blockingFirstStartRuntime{
		Runtime: base, started: make(chan agent.StartRun, 1), release: make(chan struct{}),
	}
	stateDirectory := t.TempDir()
	host, stop := runUIWithState(t, gate, "/tmp/lyra-cli-test", "ses_demo_1", stateDirectory)
	host.Shows(t, "Ask lyra")
	host.Type("FIRST_SETTLEMENT")
	host.Press(input.Enter)
	first := awaitSignalValue(t, gate.started, "accepted start held before returning its receipt")
	host.Type("SECOND_SETTLEMENT")
	host.Press(input.Enter)

	var pending []workbench.PendingRun
	host.Until(t, "both runtime commands to become durable", func() bool {
		store, err := workbench.Open(stateDirectory, workbench.Config{})
		if err != nil {
			return false
		}
		pending = store.PendingRuns(first.SessionID)
		return host.Repaint() && len(pending) == 2 && pending[0].State == workbench.PendingRunDispatching
	})
	if pending[0].Command.CommandID != first.CommandID || pending[1].Command.Message.Text != "SECOND_SETTLEMENT" {
		t.Fatalf("durable FIFO before settlement = %+v", pending)
	}

	states, err := os.ReadDir(filepath.Join(stateDirectory, "sessions"))
	if err != nil || len(states) != 1 {
		t.Fatalf("session state files = %d, %v", len(states), err)
	}
	statePath := filepath.Join(stateDirectory, "sessions", states[0].Name())
	backupPath := statePath + ".backup"
	if err := os.Rename(statePath, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(statePath, 0o700); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(statePath, "blocker")
	if err := os.WriteFile(blocker, []byte("block acknowledged start settlement"), 0o600); err != nil {
		t.Fatal(err)
	}

	close(gate.release)
	host.Shows(t, "workbench:")
	host.Shows(t, "FIRST_SETTLEMENT_RAN")
	if got := len(gate.startInputs()); got != 1 {
		t.Fatalf("second command crossed the failed settlement boundary: %d starts", got)
	}

	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backupPath, statePath); err != nil {
		t.Fatal(err)
	}
	host.Shows(t, "SECOND_SETTLEMENT_RAN")
	host.Shows(t, "complete")
	if inputs := gate.startInputs(); len(inputs) != 2 || inputs[0].Message.Text != "FIRST_SETTLEMENT" ||
		inputs[1].Message.Text != "SECOND_SETTLEMENT" {
		t.Fatalf("starts after durable recovery = %+v", inputs)
	}

	reopened, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if remaining := reopened.PendingRuns(first.SessionID); len(remaining) != 0 {
		t.Fatalf("settled FIFO remains durable: %+v", remaining)
	}
	history := reopened.History()
	if len(history) != 2 || history[0].Text != "FIRST_SETTLEMENT" || history[1].Text != "SECOND_SETTLEMENT" {
		t.Fatalf("history after settlement recovery = %+v", history)
	}
	stop()
}

func TestAcceptedStartSettlementRecoveryRestoresTheTerminalStatusWithoutAFollowUp(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockCompleted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant, Text: "ONLY_SETTLEMENT_RAN"}}},
			{Delay: time.Second, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	gate := &blockingFirstStartRuntime{
		Runtime: base, started: make(chan agent.StartRun, 1), release: make(chan struct{}),
	}
	stateDirectory := t.TempDir()
	host, stop := runUIWithState(t, gate, "/tmp/lyra-cli-test", "ses_demo_1", stateDirectory)
	host.Shows(t, "Ask lyra")
	host.Type("ONLY_SETTLEMENT")
	host.Press(input.Enter)
	awaitSignalValue(t, gate.started, "accepted start held before its single settlement")

	states, err := os.ReadDir(filepath.Join(stateDirectory, "sessions"))
	if err != nil || len(states) != 1 {
		t.Fatalf("session state files = %d, %v", len(states), err)
	}
	statePath := filepath.Join(stateDirectory, "sessions", states[0].Name())
	backupPath := statePath + ".backup"
	if err := os.Rename(statePath, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(statePath, 0o700); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(statePath, "blocker")
	if err := os.WriteFile(blocker, []byte("block single settlement"), 0o600); err != nil {
		t.Fatal(err)
	}

	close(gate.release)
	host.Shows(t, "workbench:")
	host.Shows(t, "ONLY_SETTLEMENT_RAN")
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backupPath, statePath); err != nil {
		t.Fatal(err)
	}
	host.Shows(t, "complete")
	host.Hides(t, "workbench:")
	stop()
}

func TestCancelingARunDrainsItsQueuedFollowUpAfterCancellationSettles(t *testing.T) {
	base := mock.New()
	base.Script = func(prompt string) mock.Script {
		if prompt == "CANCEL_PRIMARY" {
			return mock.Script{Prelude: []mock.Step{
				{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
			}}
		}
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockCompleted{Block: agent.Block{ID: "after-cancel", Kind: agent.BlockAssistant, Text: "QUEUED_AFTER_CANCEL_RAN"}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("CANCEL_PRIMARY")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("AFTER_CANCEL")
	host.Press(input.Enter)
	host.Shows(t, "1 queued")
	host.Press(input.Esc)
	host.Shows(t, "QUEUED_AFTER_CANCEL_RAN")
	host.Shows(t, "complete")

	inputs := backend.startInputs()
	if len(inputs) != 2 || inputs[1].Message.Text != "AFTER_CANCEL" {
		t.Fatalf("runs after cancellation = %+v", inputs)
	}
	stop()
}

func TestQueueDrawerSendsTheSelectedFollowUpBeforeTheRestAndPreservesTheDraft(t *testing.T) {
	base := mock.New()
	base.Script = func(prompt string) mock.Script {
		if prompt == "INTERRUPTED_PRIMARY" {
			return mock.Script{Prelude: []mock.Step{
				{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
			}}
		}
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockCompleted{Block: agent.Block{ID: "answer-" + prompt, Kind: agent.BlockAssistant, Text: "RAN_" + prompt}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("INTERRUPTED_PRIMARY")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("FIRST_FOLLOW_UP")
	host.Press(input.Enter)
	host.Type("SECOND_FOLLOW_UP")
	host.Press(input.Enter)
	host.Type("UNSENT_DRAFT")

	host.Send(input.Key{Code: input.Character, Rune: ';', Mods: input.Ctrl})
	host.Shows(t, "Queue · 2 prompts")
	host.Press(input.Down)
	host.Send(input.Key{Code: input.Character, Rune: 's'})
	host.Shows(t, "RAN_SECOND_FOLLOW_UP")
	host.Shows(t, "RAN_FIRST_FOLLOW_UP")
	host.Shows(t, "UNSENT_DRAFT")
	host.Hides(t, "Queue ·")

	inputs := backend.startInputs()
	if len(inputs) != 3 {
		t.Fatalf("started %d runs: %+v", len(inputs), inputs)
	}
	for index, want := range []string{"INTERRUPTED_PRIMARY", "SECOND_FOLLOW_UP", "FIRST_FOLLOW_UP"} {
		if got := inputs[index].Message.Text; got != want {
			t.Fatalf("run %d = %q, want %q", index+1, got, want)
		}
	}
	stop()
}

func TestQueueDrawerReordersAndRemovesFollowUpsBeforeDispatch(t *testing.T) {
	base := mock.New()
	base.Script = func(prompt string) mock.Script {
		if prompt == "PRIMARY_FOR_QUEUE_MUTATION" {
			return mock.Script{Prelude: []mock.Step{
				{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
			}}
		}
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockCompleted{Block: agent.Block{ID: "answer-" + prompt, Kind: agent.BlockAssistant, Text: "RAN_" + prompt}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("PRIMARY_FOR_QUEUE_MUTATION")
	host.Press(input.Enter)
	host.Shows(t, "working")
	for _, prompt := range []string{"FOLLOW_UP_ONE", "FOLLOW_UP_TWO", "FOLLOW_UP_THREE"} {
		host.Type(prompt)
		host.Press(input.Enter)
	}
	host.Shows(t, "3 queued")
	host.Send(input.Key{Code: input.Character, Rune: 'g', Mods: input.Ctrl})
	host.Shows(t, "Queue · 3 prompts")
	host.Press(input.Down)
	host.Send(input.Key{Code: input.Character, Rune: 'K', Mods: input.Shift})
	host.Shows(t, "queued prompt reordered")
	host.Press(input.Down)
	host.Send(input.Key{Code: input.Character, Rune: 'x'})
	host.Shows(t, "queued prompt removed")
	host.Press(input.Esc)
	host.Hides(t, "Queue · 2 prompts")
	host.Press(input.Esc)
	host.Shows(t, "RAN_FOLLOW_UP_TWO")
	host.Shows(t, "RAN_FOLLOW_UP_THREE")
	host.Shows(t, "complete")

	inputs := backend.startInputs()
	if len(inputs) != 3 {
		t.Fatalf("started %d runs after queue mutation: %+v", len(inputs), inputs)
	}
	for index, want := range []string{"PRIMARY_FOR_QUEUE_MUTATION", "FOLLOW_UP_TWO", "FOLLOW_UP_THREE"} {
		if got := inputs[index].Message.Text; got != want {
			t.Fatalf("run %d = %q, want %q", index+1, got, want)
		}
	}
	stop()
}

func TestEmptyEnterPromotesTheNextQueuedFollowUp(t *testing.T) {
	base := mock.New()
	base.Script = func(prompt string) mock.Script {
		if prompt == "PRIMARY_FOR_EMPTY_ENTER" {
			return mock.Script{Prelude: []mock.Step{
				{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
			}}
		}
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockCompleted{Block: agent.Block{ID: "empty-enter-answer", Kind: agent.BlockAssistant, Text: "EMPTY_ENTER_SENT_NEXT"}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("PRIMARY_FOR_EMPTY_ENTER")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("NEXT_FROM_EMPTY_ENTER")
	host.Press(input.Enter)
	host.Shows(t, "queue or send next")
	host.Press(input.Enter)
	host.Shows(t, "EMPTY_ENTER_SENT_NEXT")

	inputs := backend.startInputs()
	if len(inputs) != 2 || inputs[1].Message.Text != "NEXT_FROM_EMPTY_ENTER" {
		t.Fatalf("runs after empty Enter = %+v", inputs)
	}
	stop()
}

func TestQueueDrawerRemainsUsableOnAConstrainedTerminal(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	host, stop := runUIWith(t, base)
	host.Shows(t, "Ask lyra")
	host.Type("PRIMARY_AT_NARROW_WIDTH")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("QUEUED_AT_NARROW_WIDTH")
	host.Press(input.Enter)
	if !host.Resize(32, 10) {
		t.Fatal("resize to constrained queue layout was refused")
	}
	host.Send(input.Key{Code: input.Character, Rune: 'g', Mods: input.Ctrl})
	host.Shows(t, "Queue · 1 prompt")
	host.Press(input.Enter)
	host.Shows(t, "Editing queued prompt")
	host.Press(input.Esc)
	host.Shows(t, "edit discarded")
	host.Press(input.Esc)
	host.Hides(t, "Queue · 1 prompt")
	stop()
}

func TestEditingTheFrontPromptHoldsAutomaticDispatchUntilSave(t *testing.T) {
	base := mock.New()
	base.Script = func(prompt string) mock.Script {
		if prompt == "PRIMARY_BEFORE_QUEUE_EDIT" {
			return mock.Script{Prelude: []mock.Step{
				{Delay: 2 * time.Second, Event: agent.BlockCompleted{Block: agent.Block{ID: "primary-finished-marker", Kind: agent.BlockAssistant, Text: "PRIMARY_FINISHED_WHILE_EDITING"}}},
				{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
			}}
		}
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockCompleted{Block: agent.Block{ID: "edited-queue-answer", Kind: agent.BlockAssistant, Text: "RAN_" + prompt}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	backend := &finishObservingRuntime{
		recordingRuntime: &recordingRuntime{Runtime: base},
		finished:         make(chan struct{}),
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("PRIMARY_BEFORE_QUEUE_EDIT")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("ORIGINAL_QUEUED_TEXT")
	host.Press(input.Enter)
	host.Send(input.Key{Code: input.Character, Rune: 'g', Mods: input.Ctrl})
	host.Shows(t, "Queue · 1 prompt")
	host.Press(input.Enter)
	host.Shows(t, "Editing queued prompt")
	host.Shows(t, "PRIMARY_FINISHED_WHILE_EDITING")
	select {
	case <-backend.finished:
	case <-time.After(2 * time.Second):
		t.Fatal("primary run did not finish while the queue editor was open")
	}
	sessionID := backend.startInput().SessionID
	snapshot, err := backend.GetSession(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, active := snapshot.ActiveRun(); active {
		t.Fatal("primary run remained active after the UI presented its completed state")
	}
	if got := backend.startCount(); got != 1 {
		t.Fatalf("held queue entry started before save: %d runs", got)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'u', Mods: input.Ctrl})
	host.Type("EDITED_QUEUED_TEXT")
	host.Press(input.Enter)
	host.Shows(t, "RAN_EDITED_QUEUED_TEXT")
	inputs := backend.startInputs()
	if len(inputs) != 2 || inputs[1].Message.Text != "EDITED_QUEUED_TEXT" {
		t.Fatalf("runs after queue edit = %+v", inputs)
	}
	stop()
}

func TestQueuedFollowUpKeepsItsAttachmentIdentityUntilDispatch(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "context.txt"), []byte("queue context"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := mock.New()
	base.Script = func(prompt string) mock.Script {
		delay := time.Duration(0)
		if prompt == "ATTACHMENT_PRIMARY" {
			delay = 800 * time.Millisecond
		}
		return mock.Script{Prelude: []mock.Step{
			{Delay: delay, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWithWorkspace(t, backend, workspace)
	host.Shows(t, "Ask lyra")
	host.Type("ATTACHMENT_PRIMARY")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("/attach context.txt")
	host.Press(input.Enter)
	host.Shows(t, "attached context.txt")
	host.Type("ATTACHED_FOLLOW_UP")
	host.Press(input.Enter)
	host.Shows(t, "1 queued")
	host.Shows(t, "complete")

	inputs := backend.startInputs()
	if len(inputs) != 2 || len(inputs[1].Message.Attachments) != 1 {
		t.Fatalf("queued attachment inputs = %+v", inputs)
	}
	attachment := inputs[1].Message.Attachments[0]
	wantInfo, wantErr := os.Stat(filepath.Join(workspace, "context.txt"))
	gotInfo, gotErr := os.Stat(attachment.Path)
	if attachment.Name != "context.txt" || wantErr != nil || gotErr != nil || !os.SameFile(wantInfo, gotInfo) {
		t.Fatalf("queued attachment = %+v", attachment)
	}
	stop()
}
