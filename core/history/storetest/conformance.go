// Package storetest provides reusable conformance checks for history stores.
package storetest

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/history"
)

// Capabilities is the exact history interface set a store promises. False
// means the store must not accidentally satisfy that capability.
type Capabilities struct {
	Reader   bool
	Writer   bool
	Clearer  bool
	Lister   bool
	Replacer bool
	Counter  bool
}

// Run verifies a store's exact capability set and the validation that must
// complete before external I/O. Pass a non-nil zero-value *Store; the calls
// below must not reach provider dependencies.
func Run(t *testing.T, store any, want Capabilities) {
	t.Helper()
	if store == nil {
		t.Fatal("conformance: store must not be nil")
	}

	reader, hasReader := store.(history.Reader)
	writer, hasWriter := store.(history.Writer)
	clearer, hasClearer := store.(history.Clearer)
	lister, hasLister := store.(history.Lister)
	replacer, hasReplacer := store.(history.Replacer)
	counter, hasCounter := store.(history.Counter)

	assertCapability(t, "Reader", hasReader, want.Reader)
	assertCapability(t, "Writer", hasWriter, want.Writer)
	assertCapability(t, "Clearer", hasClearer, want.Clearer)
	assertCapability(t, "Lister", hasLister, want.Lister)
	assertCapability(t, "Replacer", hasReplacer, want.Replacer)
	assertCapability(t, "Counter", hasCounter, want.Counter)

	ctx := t.Context()
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	if want.Reader && hasReader {
		t.Run("ReadRejectsCanceledContextBeforeIO", func(t *testing.T) {
			if _, err := reader.Read(canceledCtx, "conversation"); !errors.Is(err, context.Canceled) {
				t.Fatalf("Read(canceled context) error = %v, want %v", err, context.Canceled)
			}
		})
		t.Run("ReadRejectsInvalidConversationBeforeIO", func(t *testing.T) {
			if _, err := reader.Read(ctx, ""); !errors.Is(err, history.ErrInvalidConversationID) {
				t.Fatalf("Read(empty conversation) error = %v, want %v", err, history.ErrInvalidConversationID)
			}
		})
	}
	if want.Writer && hasWriter {
		t.Run("WriteRejectsCanceledContextBeforeIO", func(t *testing.T) {
			if err := writer.Write(canceledCtx, "conversation"); !errors.Is(err, context.Canceled) {
				t.Fatalf("Write(canceled context) error = %v, want %v", err, context.Canceled)
			}
		})
		t.Run("WriteRejectsInvalidConversationBeforeIO", func(t *testing.T) {
			if err := writer.Write(ctx, ""); !errors.Is(err, history.ErrInvalidConversationID) {
				t.Fatalf("Write(empty conversation) error = %v, want %v", err, history.ErrInvalidConversationID)
			}
		})
		t.Run("WriteRejectsInvalidMessageBeforeIO", func(t *testing.T) {
			if err := writer.Write(ctx, "conversation", chat.Message{}); !errors.Is(err, chat.ErrInvalidMessage) {
				t.Fatalf("Write(invalid message) error = %v, want %v", err, chat.ErrInvalidMessage)
			}
		})
		t.Run("WriteTreatsEmptyMessagesAsNoop", func(t *testing.T) {
			if err := writer.Write(ctx, "conversation"); err != nil {
				t.Fatalf("Write(empty messages) error = %v, want nil", err)
			}
		})
	}
	if want.Clearer && hasClearer {
		t.Run("ClearRejectsCanceledContextBeforeIO", func(t *testing.T) {
			if err := clearer.Clear(canceledCtx, "conversation"); !errors.Is(err, context.Canceled) {
				t.Fatalf("Clear(canceled context) error = %v, want %v", err, context.Canceled)
			}
		})
		t.Run("ClearRejectsInvalidConversationBeforeIO", func(t *testing.T) {
			if err := clearer.Clear(ctx, ""); !errors.Is(err, history.ErrInvalidConversationID) {
				t.Fatalf("Clear(empty conversation) error = %v, want %v", err, history.ErrInvalidConversationID)
			}
		})
	}
	if want.Lister && hasLister {
		t.Run("ConversationsRejectsCanceledContextBeforeIO", func(t *testing.T) {
			if _, err := lister.Conversations(canceledCtx); !errors.Is(err, context.Canceled) {
				t.Fatalf("Conversations(canceled context) error = %v, want %v", err, context.Canceled)
			}
		})
	}
	if want.Replacer && hasReplacer {
		t.Run("ReplaceRejectsCanceledContextBeforeIO", func(t *testing.T) {
			if err := replacer.Replace(canceledCtx, "conversation"); !errors.Is(err, context.Canceled) {
				t.Fatalf("Replace(canceled context) error = %v, want %v", err, context.Canceled)
			}
		})
		t.Run("ReplaceRejectsInvalidConversationBeforeIO", func(t *testing.T) {
			if err := replacer.Replace(ctx, ""); !errors.Is(err, history.ErrInvalidConversationID) {
				t.Fatalf("Replace(empty conversation) error = %v, want %v", err, history.ErrInvalidConversationID)
			}
		})
		t.Run("ReplaceRejectsInvalidMessageBeforeIO", func(t *testing.T) {
			if err := replacer.Replace(ctx, "conversation", chat.Message{}); !errors.Is(err, chat.ErrInvalidMessage) {
				t.Fatalf("Replace(invalid message) error = %v, want %v", err, chat.ErrInvalidMessage)
			}
		})
	}
	if want.Counter && hasCounter {
		t.Run("CountRejectsCanceledContextBeforeIO", func(t *testing.T) {
			if _, err := counter.Count(canceledCtx, "conversation"); !errors.Is(err, context.Canceled) {
				t.Fatalf("Count(canceled context) error = %v, want %v", err, context.Canceled)
			}
		})
		t.Run("CountRejectsInvalidConversationBeforeIO", func(t *testing.T) {
			if _, err := counter.Count(ctx, ""); !errors.Is(err, history.ErrInvalidConversationID) {
				t.Fatalf("Count(empty conversation) error = %v, want %v", err, history.ErrInvalidConversationID)
			}
		})
	}
}

func assertCapability(t *testing.T, name string, got, want bool) {
	t.Helper()
	if got != want {
		t.Errorf("%s capability = %t, want %t", name, got, want)
	}
}
