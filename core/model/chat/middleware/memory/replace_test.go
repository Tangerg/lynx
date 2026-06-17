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
