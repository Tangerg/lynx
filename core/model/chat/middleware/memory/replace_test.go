package memory_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"
)

// TestReplace_AtomicSetsExact pins the retention-safety primitive: Replace sets
// a conversation's messages to EXACTLY the given set (not an append), and an
// empty set clears it. memory.Replace routes through the backend's Replacer
// (InMemoryStore implements it) so truncate/compaction never lose history to a
// rewrite that fails after a separate clear committed.
func TestReplace_AtomicSetsExact(t *testing.T) {
	ctx := context.Background()
	s := memory.NewInMemoryStore()
	if err := s.Write(ctx, "c",
		chat.NewUserMessage("a"), chat.NewUserMessage("b"), chat.NewUserMessage("c")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Replace with a prefix — the stored set becomes exactly that prefix.
	if err := memory.Replace(ctx, s, "c", chat.NewUserMessage("a")); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if got, _ := s.Read(ctx, "c"); len(got) != 1 {
		t.Fatalf("after Replace len = %d, want 1 (replace, not append)", len(got))
	}

	// Replace with nothing clears the conversation.
	if err := memory.Replace(ctx, s, "c"); err != nil {
		t.Fatalf("Replace empty: %v", err)
	}
	if got, _ := s.Read(ctx, "c"); len(got) != 0 {
		t.Fatalf("after empty Replace len = %d, want 0", len(got))
	}
}

// counterStore is a Store that also implements Counter, returning a canned
// count so the test can tell memory.Count dispatched to the capability rather
// than falling back to len(Read).
type counterStore struct {
	memory.Store
	n int
}

func (c counterStore) Count(context.Context, string) (int, error) { return c.n, nil }

// TestCount_PrefersCounterElseFallsBackToRead pins the optional-capability
// dispatch: memory.Count uses a backend's Counter when present (so a backend
// able to COUNT(*) doesn't materialize the whole history), and falls back to
// len(Read) for one that can't.
func TestCount_PrefersCounterElseFallsBackToRead(t *testing.T) {
	ctx := context.Background()

	// Fallback path: InMemoryStore has no Counter, so Count returns len(Read).
	plain := memory.NewInMemoryStore()
	if err := plain.Write(ctx, "c", chat.NewUserMessage("a"), chat.NewUserMessage("b")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n, err := memory.Count(ctx, plain, "c"); err != nil || n != 2 {
		t.Fatalf("fallback Count = (%d, %v), want (2, nil)", n, err)
	}

	// Capability path: a Counter-implementing store is asked directly — the
	// canned value (not the 0 messages it holds) proves Count didn't fall back.
	cs := counterStore{Store: memory.NewInMemoryStore(), n: 99}
	if n, err := memory.Count(ctx, cs, "anything"); err != nil || n != 99 {
		t.Fatalf("capability Count = (%d, %v), want (99, nil)", n, err)
	}
}
