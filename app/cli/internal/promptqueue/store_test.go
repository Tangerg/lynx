package promptqueue

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

func TestStoreKeepsSessionQueuesIsolatedAndSnapshotsDetached(t *testing.T) {
	store := New()
	first, err := store.Enqueue("one", client.Message{Text: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enqueue("two", client.Message{Text: "other session"}); err != nil {
		t.Fatal(err)
	}
	second, err := store.Enqueue("one", client.Message{Text: "second"})
	if err != nil {
		t.Fatal(err)
	}

	snapshot := store.Snapshot("one")
	if len(snapshot.Entries) != 2 || snapshot.Entries[0].ID != first.ID || snapshot.Entries[1].ID != second.ID {
		t.Fatalf("session one queue = %+v", snapshot.Entries)
	}
	snapshot.Entries[0].Message.Text = "mutated"
	if next, ok := store.Next("one"); !ok || next.Message.Text != "first" {
		t.Fatalf("snapshot mutated store: %+v, %v", next, ok)
	}
	if got := store.Snapshot("two").Entries; len(got) != 1 || got[0].Message.Text != "other session" {
		t.Fatalf("session two queue = %+v", got)
	}
}

func TestStoreUpdatesMovesRemovesAndClearsByStableIdentity(t *testing.T) {
	store := New()
	first, _ := store.Enqueue("session", client.Message{Text: "first"})
	second, _ := store.Enqueue("session", client.Message{Text: "second"})
	third, _ := store.Enqueue("session", client.Message{Text: "third"})

	if err := store.Update("session", second.ID, client.Message{Text: "edited"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Move("session", third.ID, -2); err != nil {
		t.Fatal(err)
	}
	if err := store.Move("session", third.ID, -1); !errors.Is(err, ErrMoveUnavailable) {
		t.Fatalf("moving past the front returned %v", err)
	}
	removed, err := store.Remove("session", first.ID)
	if err != nil || removed.ID != first.ID {
		t.Fatalf("removed = %+v, %v", removed, err)
	}
	got := store.Snapshot("session").Entries
	if len(got) != 2 || got[0].ID != third.ID || got[1].ID != second.ID || got[1].Message.Text != "edited" {
		t.Fatalf("queue after mutations = %+v", got)
	}
	if count := store.Clear("session"); count != 2 {
		t.Fatalf("cleared %d entries", count)
	}
	if _, ok := store.Next("session"); ok {
		t.Fatal("cleared queue still has a next entry")
	}
}

func TestStoreRejectsInvalidMessagesWithoutMutation(t *testing.T) {
	store := New()
	if _, err := store.Enqueue("", client.Message{Text: "valid"}); !errors.Is(err, ErrSessionIDRequired) {
		t.Fatalf("empty session returned %v", err)
	}
	if _, err := store.Enqueue("session", client.Message{}); err == nil {
		t.Fatal("empty message was accepted")
	}
	if snapshot := store.Snapshot("session"); len(snapshot.Entries) != 0 || snapshot.Revision != 0 {
		t.Fatalf("invalid enqueue mutated store: %+v", snapshot)
	}
}
