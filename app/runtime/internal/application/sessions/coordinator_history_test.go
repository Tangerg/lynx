package sessions

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func TestResolveForkHistoryPrefix(t *testing.T) {
	msgs := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("one")),
		chat.NewAssistantMessage(chat.NewTextPart("two")),
		chat.NewUserMessage(chat.NewTextPart("three")),
	}
	runs := []transcript.Run{
		{ID: "run_1", State: execution.Completed, CreatedAt: time.Unix(1, 0), MessageMark: 2},
		{ID: "run_2", State: execution.Completed, CreatedAt: time.Unix(3, 0), MessageMark: 3},
	}

	got, err := ResolveForkHistoryPrefix(msgs, runs, "run_1")
	if err != nil {
		t.Fatalf("resolve fork prefix: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("prefix len = %d, want 2", len(got))
	}
}

func TestResolveForkHistoryPrefixExcludesActiveTail(t *testing.T) {
	msgs := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("complete")),
		chat.NewAssistantMessage(chat.NewTextPart("boundary")),
		chat.NewUserMessage(chat.NewTextPart("active")),
	}
	runs := []transcript.Run{
		{ID: "run_1", State: execution.Completed, CreatedAt: time.Unix(1, 0), MessageMark: 2},
		{ID: "run_2", State: execution.Running, CreatedAt: time.Unix(2, 0), MessageMark: -1},
		{ID: "run_2_child", SpawnedByItemID: "item_task", State: execution.Completed, CreatedAt: time.Unix(3, 0), MessageMark: 3},
	}

	got, err := ResolveForkHistoryPrefix(msgs, runs, "")
	if err != nil {
		t.Fatalf("resolve fork prefix: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("prefix len = %d, want terminal boundary 2", len(got))
	}
}

func TestResolveForkHistoryPrefixRejectsActiveTarget(t *testing.T) {
	runs := []transcript.Run{{ID: "run_active", State: execution.Running, CreatedAt: time.Unix(1, 0), MessageMark: -1}}
	if _, err := ResolveForkHistoryPrefix([]chat.Message{chat.NewUserMessage(chat.NewTextPart("active"))}, runs, "run_active"); err != transcript.ErrRunNotFound {
		t.Fatalf("resolve active target error = %v, want ErrRunNotFound", err)
	}
}

func TestForkResolvesBoundaryFromOneCoherentSnapshot(t *testing.T) {
	reads := 0
	var plan ForkPlan
	stores := coordinatorStores{
		interrupts:    new(coordinatorInterrupts),
		snapshotReads: &reads,
		forked:        &plan,
		snapshot: Snapshot{
			Messages: []chat.Message{
				chat.NewUserMessage(chat.NewTextPart("one")),
				chat.NewAssistantMessage(chat.NewTextPart("two")),
				chat.NewUserMessage(chat.NewTextPart("three")),
			},
			Runs: []transcript.Run{
				{ID: "run_1", State: execution.Completed, CreatedAt: time.Unix(1, 0), MessageMark: 2},
				{ID: "run_2", State: execution.Completed, CreatedAt: time.Unix(2, 0), MessageMark: 3},
			},
		},
	}
	child, err := newCoordinator(stores, nil).Fork(t.Context(), ForkSpec{ParentID: "ses_1", FromRunID: "run_1"})
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if child.ID != "ses_fork" || reads != 1 || len(plan.Messages) != 2 {
		t.Fatalf("child=%+v snapshot reads=%d fork plan=%+v", child, reads, plan)
	}
}
