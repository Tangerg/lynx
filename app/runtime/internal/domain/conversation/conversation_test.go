package conversation

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

func TestConversationOwnsSequenceTransitions(t *testing.T) {
	seed := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("one")),
		chat.NewAssistantMessage(chat.NewTextPart("two")),
	}
	history, err := (Conversation{}).Seed(seed)
	if err != nil {
		t.Fatal(err)
	}
	if history.Count() != 2 {
		t.Fatalf("count = %d, want 2", history.Count())
	}
	if _, seedErr := history.Seed(seed); !errors.Is(seedErr, ErrNotEmpty) {
		t.Fatalf("second seed error = %v, want ErrNotEmpty", seedErr)
	}
	history = history.Truncate(1)
	history, err = history.Append(chat.NewAssistantMessage(chat.NewTextPart("four")))
	if err != nil {
		t.Fatal(err)
	}
	messages := history.Messages()
	if len(messages) != 2 || messages[0].Text() != "one" || messages[1].Text() != "four" {
		t.Fatalf("messages = %#v", messages)
	}
	messages[0] = chat.NewUserMessage(chat.NewTextPart("changed"))
	if history.Messages()[0].Text() != "one" {
		t.Fatal("Messages leaked aggregate ownership")
	}
}

func TestCloseOpenToolCallsClosesOnlyLatestUnresolvedGenerations(t *testing.T) {
	history, err := New([]chat.Message{
		chat.NewAssistantMessage(
			chat.NewToolCallPart(chat.ToolCall{ID: "call_reused", Name: "first"}),
			chat.NewToolCallPart(chat.ToolCall{ID: "call_parallel", Name: "parallel"}),
		),
		chat.NewToolMessage(chat.ToolResult{ID: "call_reused", Name: "first", Result: "done"}),
		chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "call_reused", Name: "second"})),
	})
	if err != nil {
		t.Fatal(err)
	}
	closed, appended, err := history.CloseOpenToolCalls("execution was lost")
	if err != nil {
		t.Fatal(err)
	}
	if history.Count() != 3 || closed.Count() != 4 || len(appended) != 1 {
		t.Fatalf("counts/appended = %d/%d/%d, want 3/4/1", history.Count(), closed.Count(), len(appended))
	}
	want := chat.NewToolMessage(
		chat.ToolResult{ID: "call_parallel", Name: "parallel", Result: "execution was lost", IsError: true},
		chat.ToolResult{ID: "call_reused", Name: "second", Result: "execution was lost", IsError: true},
	)
	if !reflect.DeepEqual(appended[0], want) || !reflect.DeepEqual(closed.Messages()[3], want) {
		t.Fatalf("closure = %#v, want %#v", appended, want)
	}
	appended[0] = chat.NewToolMessage(chat.ToolResult{ID: "changed", Name: "changed"})
	if !reflect.DeepEqual(closed.Messages()[3], want) {
		t.Fatal("CloseOpenToolCalls leaked aggregate ownership")
	}
}

func TestCloseOpenToolCallsIsNoOpWhenConversationIsClosed(t *testing.T) {
	history, err := New([]chat.Message{
		chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "call", Name: "read"})),
		chat.NewToolMessage(chat.ToolResult{ID: "call", Name: "read", Result: "done"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	closed, appended, err := history.CloseOpenToolCalls("unused")
	if err != nil || len(appended) != 0 || !reflect.DeepEqual(closed.Messages(), history.Messages()) {
		t.Fatalf("closed/appended/error = %#v/%#v/%v", closed.Messages(), appended, err)
	}
}

func TestCloseOpenToolCallsWithResultsPreservesProviderOrder(t *testing.T) {
	history, err := New([]chat.Message{chat.NewAssistantMessage(
		chat.NewToolCallPart(chat.ToolCall{ID: "first", Name: "read", Arguments: `{}`}),
		chat.NewToolCallPart(chat.ToolCall{ID: "second", Name: "glob", Arguments: `{}`}),
	)})
	if err != nil {
		t.Fatal(err)
	}
	closed, appended, err := history.CloseOpenToolCallsWithResults(
		"canceled",
		[]chat.ToolResult{{ID: "second", Name: "glob", Result: "known"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Count() != 2 || len(appended) != 1 || len(appended[0].Parts) != 2 {
		t.Fatalf("closed/appended = %d/%#v", closed.Count(), appended)
	}
	first := appended[0].Parts[0].ToolResult
	second := appended[0].Parts[1].ToolResult
	if first == nil || first.ID != "first" || !first.IsError || first.Result != "canceled" ||
		second == nil || second.ID != "second" || second.IsError || second.Result != "known" {
		t.Fatalf("ordered terminal results = %#v", appended[0].Parts)
	}
}
