package maintenance

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/chatclient"
	history "github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
)

func TestLiveStateReminderRendersShellsAndTodos(t *testing.T) {
	msg, ok := liveStateReminder(LiveStateSnapshot{
		Shells: []RunningShell{{ID: "bg_1", Command: "npm run dev"}},
		Todos:  []string{"Wire the search index"},
	})
	if !ok {
		t.Fatal("non-empty snapshot should render a reminder")
	}
	body := msg.Text()
	for _, want := range []string{"<system-reminder>", "bg_1", "npm run dev", "shell_output", "Wire the search index"} {
		if !strings.Contains(body, want) {
			t.Fatalf("reminder missing %q:\n%s", want, body)
		}
	}
}

func TestLiveStateReminderEmptyIsSkipped(t *testing.T) {
	if _, ok := liveStateReminder(LiveStateSnapshot{}); ok {
		t.Fatal("empty snapshot must not render a reminder")
	}
}

// TestCompactorAppendsLiveStateReminder drives the full summary rung and asserts
// the reminder lands right after the summary, ahead of the kept recent slice.
func TestCompactorAppendsLiveStateReminder(t *testing.T) {
	store := history.NewInMemoryStore()
	const sessID = "sess-live"
	const total = 20
	for range total {
		_ = store.Write(context.Background(), sessID, chat.NewUserMessage(chat.NewTextPart("msg")))
	}
	client, _ := chatclient.New(newTextStubModel("BULLETS"))

	live := func(_ context.Context, id string) LiveStateSnapshot {
		if id != sessID {
			t.Errorf("live-state queried for %q, want %q", id, sessID)
		}
		return LiveStateSnapshot{
			Shells: []RunningShell{{ID: "bg_7", Command: "go test ./..."}},
			Todos:  []string{"Finish the compaction reminder"},
		}
	}

	c := NewCompactor(store, constClient(client), live, CompactionConfig{MaxMessages: total, KeepRecent: 4})
	res, err := c.MaybeCompact(context.Background(), sessID, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Compacted {
		t.Fatal("expected compaction to fire")
	}

	after, _ := store.Read(context.Background(), sessID)
	// [summary, reminder, ...4 recent]
	if len(after) != 6 {
		t.Fatalf("post-compact len = %d, want 6 (summary + reminder + 4)", len(after))
	}
	if !strings.HasPrefix(after[0].Text(), "[Earlier conversation summary]") {
		t.Fatalf("after[0] should be the summary, got %q", after[0].Text())
	}
	reminder := after[1].Text()
	if !strings.Contains(reminder, "<system-reminder>") || !strings.Contains(reminder, "bg_7") ||
		!strings.Contains(reminder, "Finish the compaction reminder") {
		t.Fatalf("after[1] should be the live-state reminder, got %q", reminder)
	}
}

// TestCompactorSkipsReminderWhenNoLiveState confirms an empty snapshot leaves the
// rewritten history exactly [summary, ...recent] — no stray reminder message.
func TestCompactorSkipsReminderWhenNoLiveState(t *testing.T) {
	store := history.NewInMemoryStore()
	const sessID = "sess-live-empty"
	const total = 20
	for range total {
		_ = store.Write(context.Background(), sessID, chat.NewUserMessage(chat.NewTextPart("msg")))
	}
	client, _ := chatclient.New(newTextStubModel("BULLETS"))

	live := func(context.Context, string) LiveStateSnapshot { return LiveStateSnapshot{} }
	c := NewCompactor(store, constClient(client), live, CompactionConfig{MaxMessages: total, KeepRecent: 4})
	if _, err := c.MaybeCompact(context.Background(), sessID, 0, nil); err != nil {
		t.Fatal(err)
	}
	after, _ := store.Read(context.Background(), sessID)
	if len(after) != 5 {
		t.Fatalf("empty live-state should leave summary + 4 recent = 5, got %d", len(after))
	}
	for _, m := range after {
		if strings.Contains(m.Text(), "<system-reminder>") {
			t.Fatalf("no reminder should be injected for an empty snapshot: %q", m.Text())
		}
	}
}
