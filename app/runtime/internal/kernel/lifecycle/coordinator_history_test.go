package lifecycle

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

func TestResolveForkHistoryPrefix(t *testing.T) {
	msgs := []chat.Message{
		chat.NewUserMessage("one"),
		chat.NewAssistantMessage("two"),
		chat.NewUserMessage("three"),
	}
	nodes := []transcript.RunNode{
		{ID: "run_1", CreatedAt: time.Unix(1, 0), Mark: 1},
		{ID: "run_1_resume", ParentRunID: "run_1", CreatedAt: time.Unix(2, 0), Mark: 2},
		{ID: "run_2", CreatedAt: time.Unix(3, 0), Mark: 3},
	}

	got, err := ResolveForkHistoryPrefix(msgs, nodes, "run_1_resume")
	if err != nil {
		t.Fatalf("resolve fork prefix: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("prefix len = %d, want 2", len(got))
	}
}

func TestResolveForkHistoryPrefixKeepsFullHistoryOnUnknownMark(t *testing.T) {
	msgs := []chat.Message{chat.NewUserMessage("one"), chat.NewAssistantMessage("two")}
	nodes := []transcript.RunNode{{ID: "run_1", CreatedAt: time.Unix(1, 0), Mark: -1}}

	got, err := ResolveForkHistoryPrefix(msgs, nodes, "run_1")
	if err != nil {
		t.Fatalf("resolve fork prefix: %v", err)
	}
	if len(got) != len(msgs) {
		t.Fatalf("prefix len = %d, want full history %d", len(got), len(msgs))
	}
}
