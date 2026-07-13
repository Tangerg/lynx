package sessions

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func TestResolveForkHistoryPrefix(t *testing.T) {
	msgs := []chat.Message{
		chat.NewUserMessage("one"),
		chat.NewAssistantMessage("two"),
		chat.NewUserMessage("three"),
	}
	nodes := []transcript.RunNode{
		{ID: "run_1", CreatedAt: time.Unix(1, 0), Mark: 2},
		{ID: "run_2", CreatedAt: time.Unix(3, 0), Mark: 3},
	}

	got, err := ResolveForkHistoryPrefix(msgs, nodes, "run_1")
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

func TestForkResolvesBoundaryFromOneCoherentSnapshot(t *testing.T) {
	reads := 0
	var plan ForkPlan
	stores := coordinatorStores{
		interrupts:    new(coordinatorInterrupts),
		snapshotReads: &reads,
		forked:        &plan,
		snapshot: Snapshot{
			Messages: []chat.Message{
				chat.NewUserMessage("one"),
				chat.NewAssistantMessage("two"),
				chat.NewUserMessage("three"),
			},
			Runs: []transcript.Run{
				{ID: "run_1", CreatedAt: time.Unix(1, 0), MessageMark: 2},
				{ID: "run_2", CreatedAt: time.Unix(2, 0), MessageMark: 3},
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
