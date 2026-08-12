package promptqueue

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestQueueKeepsSessionQueuesIsolatedAndSnapshotsDetached(t *testing.T) {
	queue := New()
	first, err := queue.Enqueue("one", agent.Message{Text: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Enqueue("two", agent.Message{Text: "other session"}); err != nil {
		t.Fatal(err)
	}
	second, err := queue.Enqueue("one", agent.Message{Text: "second"})
	if err != nil {
		t.Fatal(err)
	}

	snapshot := queue.Snapshot("one")
	if len(snapshot.Entries) != 2 || snapshot.Entries[0].ID != first.ID || snapshot.Entries[1].ID != second.ID {
		t.Fatalf("session one queue = %+v", snapshot.Entries)
	}
	snapshot.Entries[0].Message.Text = "mutated"
	if current := queue.Snapshot("one").Entries; len(current) != 2 || current[0].Message.Text != "first" {
		t.Fatalf("snapshot mutated queue: %+v", current)
	}
	if got := queue.Snapshot("two").Entries; len(got) != 1 || got[0].Message.Text != "other session" {
		t.Fatalf("session two queue = %+v", got)
	}
}

func TestQueueUpdatesMovesRemovesAndClearsByStableIdentity(t *testing.T) {
	queue := New()
	first, _ := queue.Enqueue("session", agent.Message{Text: "first"})
	second, _ := queue.Enqueue("session", agent.Message{Text: "second"})
	third, _ := queue.Enqueue("session", agent.Message{Text: "third"})

	if err := queue.Update("session", second.ID, agent.Message{Text: "edited"}); err != nil {
		t.Fatal(err)
	}
	if err := queue.Move("session", third.ID, -2); err != nil {
		t.Fatal(err)
	}
	if err := queue.Move("session", third.ID, -1); !errors.Is(err, ErrMoveUnavailable) {
		t.Fatalf("moving past the front returned %v", err)
	}
	removed, err := queue.Remove("session", first.ID)
	if err != nil || removed.ID != first.ID {
		t.Fatalf("removed = %+v, %v", removed, err)
	}
	got := queue.Snapshot("session").Entries
	if len(got) != 2 || got[0].ID != third.ID || got[1].ID != second.ID || got[1].Message.Text != "edited" {
		t.Fatalf("queue after mutations = %+v", got)
	}
	if count := queue.Clear("session"); count != 2 {
		t.Fatalf("cleared %d entries", count)
	}
	if snapshot := queue.Snapshot("session"); len(snapshot.Entries) != 0 {
		t.Fatalf("cleared queue still has entries: %+v", snapshot)
	}
}

func TestQueuePromotesAnEntryWithoutChangingItsIdentity(t *testing.T) {
	queue := New()
	first, _ := queue.Enqueue("session", agent.Message{Text: "first"})
	second, _ := queue.Enqueue("session", agent.Message{Text: "second"})
	third, _ := queue.Enqueue("session", agent.Message{Text: "third"})

	if err := queue.Promote("session", third.ID); err != nil {
		t.Fatal(err)
	}
	got := queue.Snapshot("session").Entries
	if len(got) != 3 || got[0].ID != third.ID || got[1].ID != first.ID || got[2].ID != second.ID {
		t.Fatalf("promoted queue = %+v", got)
	}
	revision := queue.Snapshot("session").Revision
	if err := queue.Promote("session", third.ID); err != nil {
		t.Fatal(err)
	}
	if got := queue.Snapshot("session").Revision; got != revision {
		t.Fatalf("promoting the front entry changed revision from %d to %d", revision, got)
	}
}

func TestQueueHoldsTheFrontEntryUntilEditingReleasesIt(t *testing.T) {
	queue := New()
	first, _ := queue.Enqueue("session", agent.Message{Text: "first"})
	second, _ := queue.Enqueue("session", agent.Message{Text: "second"})
	if err := queue.Hold("session", first.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := queue.BeginDispatch("session"); ok {
		t.Fatal("held front entry was dispatchable")
	}
	snapshot := queue.Snapshot("session")
	if len(snapshot.Entries) != 2 || !snapshot.Entries[0].Held || snapshot.Entries[1].ID != second.ID {
		t.Fatalf("held snapshot = %+v", snapshot.Entries)
	}
	if err := queue.Hold("session", first.ID); !errors.Is(err, ErrEntryHeld) {
		t.Fatalf("second hold returned %v", err)
	}
	if err := queue.Release("session", first.ID); err != nil {
		t.Fatal(err)
	}
	if err := queue.Release("session", first.ID); err != nil {
		t.Fatalf("idempotent release returned %v", err)
	}
	if next, ok := queue.BeginDispatch("session"); !ok || next.ID != first.ID || next.Held {
		t.Fatalf("released next entry = %+v, %v", next, ok)
	}
	queue.ReleaseDispatch("session")
}

func TestQueueRejectsInvalidMessagesWithoutMutation(t *testing.T) {
	queue := New()
	if _, err := queue.Enqueue("", agent.Message{Text: "valid"}); !errors.Is(err, ErrSessionIDRequired) {
		t.Fatalf("empty session returned %v", err)
	}
	if _, err := queue.Enqueue("session", agent.Message{}); err == nil {
		t.Fatal("empty message was accepted")
	}
	if snapshot := queue.Snapshot("session"); len(snapshot.Entries) != 0 || snapshot.Revision != 0 {
		t.Fatalf("invalid enqueue mutated queue: %+v", snapshot)
	}
}

func TestQueueRejectsInvalidRunOptionsWithoutMutation(t *testing.T) {
	queue := New()
	existing, err := queue.Enqueue("session", agent.Message{Text: "existing"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := queue.BeginDispatch("session"); !ok {
		t.Fatal("queue did not reserve its existing entry")
	}
	before := queue.State("session")
	invalid := agent.RunOptions{Provider: "deepseek"}
	command := agent.StartRun{
		CommandID: agent.CommandID("cli_11111111111111111111111111111111"),
		SessionID: "session", Message: agent.Message{Text: "invalid"}, Options: invalid,
	}
	if _, err := queue.EnqueueCommand(command.CommandID, command.SessionID, command.Message, command.Options); err == nil {
		t.Fatal("queue accepted invalid run options")
	}
	if err := queue.Restore("session", []agent.StartRun{command}, command.CommandID); err == nil {
		t.Fatal("durable restore accepted invalid run options")
	}
	state := State{Entries: []Entry{{
		ID: 99, CommandID: command.CommandID, SessionID: command.SessionID,
		Message: command.Message, Options: command.Options,
	}}, DispatchingID: 99}
	if err := queue.RestoreState("session", state); err == nil {
		t.Fatal("transaction restore accepted invalid run options")
	}
	after := queue.State("session")
	if len(after.Entries) != 1 || after.Entries[0].ID != existing.ID || after.DispatchingID != before.DispatchingID ||
		after.Entries[0].CommandID != before.Entries[0].CommandID {
		t.Fatalf("invalid options mutated queue: before=%+v after=%+v", before, after)
	}
}

func TestQueueRestoresAnExactSnapshotAfterARejectedTransaction(t *testing.T) {
	queue := New()
	first, _ := queue.Enqueue("session", agent.Message{Text: "first"})
	second, _ := queue.Enqueue("session", agent.Message{Text: "second"})
	if err := queue.Hold("session", first.ID); err != nil {
		t.Fatal(err)
	}
	before := queue.State("session")

	if err := queue.Update("session", first.ID, agent.Message{Text: "edited"}); err != nil {
		t.Fatal(err)
	}
	if err := queue.Release("session", first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Remove("session", second.ID); err != nil {
		t.Fatal(err)
	}
	if err := queue.RestoreState("session", before); err != nil {
		t.Fatal(err)
	}

	after := queue.Snapshot("session")
	if len(after.Entries) != 2 || after.Entries[0].ID != first.ID || after.Entries[1].ID != second.ID {
		t.Fatalf("restored queue = %+v", after.Entries)
	}
	if after.Entries[0].CommandID != first.CommandID || after.Entries[0].Message.Text != "first" || !after.Entries[0].Held {
		t.Fatalf("restored first entry = %+v", after.Entries[0])
	}
	next, err := queue.Enqueue("session", agent.Message{Text: "third"})
	if err != nil {
		t.Fatal(err)
	}
	if next.ID <= second.ID {
		t.Fatalf("new entry reused identity %d after restoring through %d", next.ID, second.ID)
	}
}

func TestQueueRestoresDurableCommandsWithFreshLocalIdentities(t *testing.T) {
	queue := New()
	if _, err := queue.Enqueue("other", agent.Message{Text: "advance local identity"}); err != nil {
		t.Fatal(err)
	}
	commands := []agent.StartRun{
		{CommandID: agent.CommandID("cli_11111111111111111111111111111111"), SessionID: "session", Message: agent.Message{Text: "first"}},
		{CommandID: agent.CommandID("cli_22222222222222222222222222222222"), SessionID: "session", Message: agent.Message{Text: "second"}},
	}
	if err := queue.Restore("session", commands, ""); err != nil {
		t.Fatal(err)
	}
	snapshot := queue.Snapshot("session")
	if len(snapshot.Entries) != 2 || snapshot.Entries[0].ID == 0 ||
		snapshot.Entries[0].ID >= snapshot.Entries[1].ID || snapshot.Entries[0].CommandID != commands[0].CommandID ||
		snapshot.Entries[1].Message.Text != commands[1].Message.Text {
		t.Fatalf("restored durable queue = %+v", snapshot)
	}
	if err := queue.Restore("session", nil, ""); err != nil {
		t.Fatal(err)
	}
	if got := queue.State("session"); len(got.Entries) != 0 || got.DispatchingID != 0 {
		t.Fatalf("empty restore left queue state: %+v", got)
	}
}

func TestQueueRestoresADurableDispatchReservationAtomically(t *testing.T) {
	queue := New()
	commands := []agent.StartRun{
		{CommandID: agent.CommandID("cli_11111111111111111111111111111111"), SessionID: "session", Message: agent.Message{Text: "opening"}},
		{CommandID: agent.CommandID("cli_22222222222222222222222222222222"), SessionID: "session", Message: agent.Message{Text: "queued"}},
	}
	if err := queue.Restore("session", commands, commands[0].CommandID); err != nil {
		t.Fatal(err)
	}
	dispatching, ok := queue.Dispatching("session")
	if !ok || dispatching.CommandID != commands[0].CommandID {
		t.Fatalf("restored dispatch = %+v, %t", dispatching, ok)
	}
	if _, err := queue.Remove("session", dispatching.ID); !errors.Is(err, ErrEntryDispatching) {
		t.Fatalf("restored dispatch removal returned %v", err)
	}
	if err := queue.Promote("session", queue.Snapshot("session").Entries[1].ID); err != nil {
		t.Fatal(err)
	}
	if first := queue.Snapshot("session").Entries[0]; first.CommandID != commands[0].CommandID {
		t.Fatalf("priority edit crossed restored dispatch: %+v", queue.State("session"))
	}
}

func TestQueueRejectsAnInvalidDurableDispatchWithoutMutation(t *testing.T) {
	queue := New()
	existing, _ := queue.Enqueue("session", agent.Message{Text: "existing"})
	before := queue.State("session")
	commands := []agent.StartRun{
		{CommandID: agent.CommandID("cli_11111111111111111111111111111111"), SessionID: "session", Message: agent.Message{Text: "first"}},
		{CommandID: agent.CommandID("cli_22222222222222222222222222222222"), SessionID: "session", Message: agent.Message{Text: "second"}},
	}
	if err := queue.Restore("session", commands, commands[1].CommandID); err == nil {
		t.Fatal("queue accepted a non-front durable dispatch")
	}
	after := queue.State("session")
	if len(after.Entries) != 1 || after.Entries[0].ID != existing.ID || after.Entries[0].CommandID != before.Entries[0].CommandID ||
		after.DispatchingID != before.DispatchingID {
		t.Fatalf("invalid durable restore mutated queue: before=%+v after=%+v", before, after)
	}
}

func TestDispatchReservationProtectsRuntimeIdentityFromPriorityEdits(t *testing.T) {
	queue := New()
	first, _ := queue.Enqueue("session", agent.Message{Text: "opening"})
	second, _ := queue.Enqueue("session", agent.Message{Text: "send next"})
	third, _ := queue.Enqueue("session", agent.Message{Text: "leave last"})

	dispatching, ok := queue.BeginDispatch("session")
	if !ok || dispatching.ID != first.ID {
		t.Fatalf("dispatch reservation = %+v, %t", dispatching, ok)
	}
	if err := queue.Promote("session", third.ID); err != nil {
		t.Fatal(err)
	}
	entries := queue.Snapshot("session").Entries
	if len(entries) != 3 || entries[0].ID != first.ID || queue.State("session").DispatchingID != first.ID ||
		entries[1].ID != third.ID || entries[2].ID != second.ID {
		t.Fatalf("priority edit crossed dispatch boundary: %+v", entries)
	}
	if _, ok := queue.BeginDispatch("session"); ok {
		t.Fatal("queue exposed a second dispatch while the first was reserved")
	}
	if _, err := queue.Remove("session", first.ID); !errors.Is(err, ErrEntryDispatching) {
		t.Fatalf("dispatching removal returned %v", err)
	}
	if err := queue.Move("session", third.ID, -1); !errors.Is(err, ErrMoveUnavailable) {
		t.Fatalf("move across dispatch boundary returned %v", err)
	}

	removed, err := queue.CommitDispatch("session")
	if err != nil || removed.ID != first.ID {
		t.Fatalf("committed dispatch = %+v, %v", removed, err)
	}
	if next, ok := queue.BeginDispatch("session"); !ok || next.ID != third.ID {
		t.Fatalf("next after dispatch = %+v, %t", next, ok)
	}
}

func TestRestoreStatePreservesDispatchReservation(t *testing.T) {
	queue := New()
	first, _ := queue.Enqueue("session", agent.Message{Text: "opening"})
	second, _ := queue.Enqueue("session", agent.Message{Text: "queued"})
	if _, ok := queue.BeginDispatch("session"); !ok {
		t.Fatal("could not reserve the front entry")
	}
	before := queue.State("session")

	if _, err := queue.RetireCommand("session", first.CommandID); err != nil {
		t.Fatal(err)
	}
	if err := queue.RestoreState("session", before); err != nil {
		t.Fatal(err)
	}
	dispatching, ok := queue.Dispatching("session")
	if !ok || dispatching.ID != first.ID || dispatching.CommandID != first.CommandID {
		t.Fatalf("restored dispatch = %+v, %t", dispatching, ok)
	}
	entries := queue.Snapshot("session").Entries
	if len(entries) != 2 || entries[1].ID != second.ID || queue.State("session").DispatchingID != first.ID {
		t.Fatalf("restored snapshot = %+v", entries)
	}
}

func TestRestoreStateRejectsInvalidDispatchWithoutChangingTheQueue(t *testing.T) {
	queue := New()
	first, _ := queue.Enqueue("session", agent.Message{Text: "first"})
	second, _ := queue.Enqueue("session", agent.Message{Text: "second"})
	before := queue.State("session")
	invalid := before
	invalid.DispatchingID = second.ID

	if err := queue.RestoreState("session", invalid); err == nil {
		t.Fatal("snapshot reserved a non-front entry")
	}
	after := queue.State("session")
	if after.DispatchingID != before.DispatchingID || len(after.Entries) != 2 ||
		after.Entries[0].ID != first.ID || after.Entries[1].ID != second.ID {
		t.Fatalf("invalid restore mutated queue: before=%+v after=%+v", before, after)
	}
}

func TestRejectedDispatchIsReidentifiedAndReleasedAtomically(t *testing.T) {
	queue := New()
	first, _ := queue.Enqueue("session", agent.Message{Text: "retry me"})
	if _, ok := queue.BeginDispatch("session"); !ok {
		t.Fatal("could not reserve dispatch")
	}
	replacement := agent.CommandID("cli_99999999999999999999999999999999")
	if err := queue.RequeueDispatch("session", first.CommandID, replacement); err != nil {
		t.Fatal(err)
	}
	if queue.State("session").DispatchingID != 0 {
		t.Fatal("requeued dispatch retained its reservation")
	}
	next, ok := queue.BeginDispatch("session")
	if !ok || next.ID != first.ID || next.CommandID != replacement {
		t.Fatalf("requeued dispatch = %+v, %t", next, ok)
	}
}

func TestReleasingDispatchReturnsTheSameCommandToFIFO(t *testing.T) {
	queue := New()
	first, _ := queue.Enqueue("session", agent.Message{Text: "opening"})
	if queue.ReleaseDispatch("session") {
		t.Fatal("empty dispatch release reported a state change")
	}
	if _, ok := queue.BeginDispatch("session"); !ok {
		t.Fatal("could not reserve dispatch")
	}
	if !queue.ReleaseDispatch("session") {
		t.Fatal("dispatch release reported no state change")
	}
	if next, ok := queue.BeginDispatch("session"); !ok || next.ID != first.ID || next.CommandID != first.CommandID {
		t.Fatalf("released FIFO entry = %+v, %t", next, ok)
	}
}
