package todotool

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/lyra/internal/service/todo"
)

type stubStore struct{ items []todo.Item }

func (s *stubStore) List(context.Context, string) ([]todo.Item, error) { return s.items, nil }
func (s *stubStore) Replace(_ context.Context, _ string, items []todo.Item) error {
	s.items = items
	return nil
}

func TestNew_NilServiceOmitted(t *testing.T) {
	if New(nil) != nil {
		t.Fatal("New(nil) should return nil so the caller omits the tool (feature disabled)")
	}
}

func TestTodoWrite_Definition(t *testing.T) {
	tool := New(&stubStore{})
	if tool == nil {
		t.Fatal("New(svc) returned nil")
	}
	if got := tool.Definition().Name; got != "todo_write" {
		t.Fatalf("tool name = %q, want todo_write", got)
	}
}

// TestTodoWrite_NoSession verifies the tool refuses cleanly (a recoverable
// message, never an error) when the turn carries no session — a bare context
// has no process/blackboard, so [turnctx.TurnSession] yields "". The
// validate/persist/render path keys off the domain + store, covered by their
// own tests; injecting a session here would need full process/blackboard
// plumbing no other toolset test sets up.
func TestTodoWrite_NoSession(t *testing.T) {
	tool := New(&stubStore{})
	out, err := tool.Call(context.Background(), `{"todos":[{"content":"a","status":"pending"}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "no active session") {
		t.Fatalf("output = %q, want a no-session message", out)
	}
}

func TestTodoWrite_BadArguments(t *testing.T) {
	tool := New(&stubStore{})
	out, err := tool.Call(context.Background(), `{not json`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "invalid arguments") {
		t.Fatalf("output = %q, want an invalid-arguments message", out)
	}
}
