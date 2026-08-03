package sessions

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func TestResolveForkBoundary(t *testing.T) {
	msgs := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("one")),
		chat.NewAssistantMessage(chat.NewTextPart("two")),
		chat.NewUserMessage(chat.NewTextPart("three")),
	}
	runs := []transcript.Run{
		{ID: "run_1", State: execution.Completed, CreatedAt: time.Unix(1, 0), MessageMark: 2},
		{ID: "run_2", State: execution.Completed, CreatedAt: time.Unix(3, 0), MessageMark: 3},
	}

	got, err := ResolveForkBoundary(msgs, runs, "run_1")
	if err != nil {
		t.Fatalf("resolve fork boundary: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("prefix len = %d, want 2", len(got.Messages))
	}
	// The prefix and the state the child inherits must name the same run, or a branch
	// gets a Plan its conversation never produced.
	if got.RunID != "run_1" {
		t.Fatalf("boundary run = %q, want run_1", got.RunID)
	}
}

func TestResolveForkBoundaryExcludesActiveTail(t *testing.T) {
	msgs := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("complete")),
		chat.NewAssistantMessage(chat.NewTextPart("boundary")),
		chat.NewUserMessage(chat.NewTextPart("active")),
	}
	runs := []transcript.Run{
		{ID: "run_1", State: execution.Completed, CreatedAt: time.Unix(1, 0), MessageMark: 2},
		{ID: "run_2", State: execution.Running, CreatedAt: time.Unix(2, 0), MessageMark: -1},
		{
			ID: "run_2_child", SpawnedByItemID: "item_task",
			ParentRunID: "run_2", RootRunID: "run_2",
			State: execution.Completed, CreatedAt: time.Unix(3, 0), MessageMark: 3,
		},
	}

	got, err := ResolveForkBoundary(msgs, runs, "")
	if err != nil {
		t.Fatalf("resolve fork boundary: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("prefix len = %d, want terminal boundary 2", len(got.Messages))
	}
	if got.RunID != "run_1" {
		t.Fatalf("boundary run = %q, want the last terminal run run_1", got.RunID)
	}
}

func TestResolveForkBoundaryRejectsActiveTarget(t *testing.T) {
	runs := []transcript.Run{{ID: "run_active", State: execution.Running, CreatedAt: time.Unix(1, 0), MessageMark: -1}}
	if _, err := ResolveForkBoundary([]chat.Message{chat.NewUserMessage(chat.NewTextPart("active"))}, runs, "run_active"); err != transcript.ErrRunNotFound {
		t.Fatalf("resolve active target error = %v, want ErrRunNotFound", err)
	}
}
