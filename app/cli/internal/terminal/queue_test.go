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
