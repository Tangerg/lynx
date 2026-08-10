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
	if next, ok := queue.Next("one"); !ok || next.Message.Text != "first" {
		t.Fatalf("snapshot mutated queue: %+v, %v", next, ok)
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
	if _, ok := queue.Next("session"); ok {
		t.Fatal("cleared queue still has a next entry")
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
	if _, ok := queue.Next("session"); ok {
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
	if next, ok := queue.Next("session"); !ok || next.ID != first.ID || next.Held {
		t.Fatalf("released next entry = %+v, %v", next, ok)
	}
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
