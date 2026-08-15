package conversations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/conversation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
	"github.com/Tangerg/lynx/chathistory/inmemory"
	"github.com/Tangerg/lynx/core/chat"
)

type recordingCompactions struct {
	runs []run.Run
	plan CompactionPlan
}

func (s *recordingCompactions) ListRuns(context.Context, string) ([]run.Run, error) {
	return append([]run.Run(nil), s.runs...), nil
}

func (s *recordingCompactions) ApplyCompaction(_ context.Context, plan CompactionPlan) error {
	s.plan = plan
	return nil
}

func TestMessagesCoordinatesDurableHistory(t *testing.T) {
	messages := NewMessages(inmemory.New(), nil)
	seed := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("one")),
		chat.NewAssistantMessage(chat.NewTextPart("two")),
		chat.NewUserMessage(chat.NewTextPart("three")),
	}
	if err := messages.Seed(t.Context(), "ses_1", seed); err != nil {
		t.Fatal(err)
	}
	if err := messages.Seed(t.Context(), "ses_1", seed); !errors.Is(err, conversation.ErrNotEmpty) {
		t.Fatalf("second seed error = %v", err)
	}
	if err := messages.Truncate(t.Context(), "ses_1", 2); err != nil {
		t.Fatal(err)
	}
	if err := messages.Append(t.Context(), "ses_1", chat.NewUserMessage(chat.NewTextPart("four"))); err != nil {
		t.Fatal(err)
	}
	got, err := messages.Read(t.Context(), "ses_1")
	if err != nil || len(got) != 3 || got[1].Text() != "two" || got[2].Text() != "four" {
		t.Fatalf("Read = %#v, %v", got, err)
	}
}

func TestMessagesRejectsMissingSession(t *testing.T) {
	messages := NewMessages(inmemory.New(), nil)
	if _, err := messages.Read(t.Context(), ""); !errors.Is(err, errSessionIDRequired) {
		t.Fatalf("Read error = %v", err)
	}
	if err := messages.Append(t.Context(), "", chat.NewUserMessage(chat.NewTextPart("one"))); !errors.Is(err, errSessionIDRequired) {
		t.Fatalf("Append error = %v", err)
	}
}

func TestMessagesPlansCompactionRunWatermarks(t *testing.T) {
	at := time.Unix(10, 0).UTC()
	compactions := &recordingCompactions{runs: []run.Run{
		runfixture.MustRestore(run.Snapshot{ID: "run_before", SessionID: "ses_1", State: run.Completed, CreatedAt: at, MessageMark: 4}),
		runfixture.MustRestore(run.Snapshot{ID: "run_cut", SessionID: "ses_1", State: run.Completed, CreatedAt: at.Add(time.Second), MessageMark: 6}),
		runfixture.MustRestore(run.Snapshot{ID: "run_recent", SessionID: "ses_1", State: run.Completed, CreatedAt: at.Add(2 * time.Second), MessageMark: 8}),
		runfixture.MustRestore(run.Snapshot{ID: "run_active", SessionID: "ses_1", State: run.Running, CreatedAt: at.Add(3 * time.Second)}),
	}}
	messages := NewMessages(inmemory.New(), compactions)
	replacement := []chat.Message{
		chat.NewSystemMessage("summary"),
		chat.NewUserMessage(chat.NewTextPart("recent question")),
		chat.NewAssistantMessage(chat.NewTextPart("recent answer")),
	}
	if err := messages.RewriteForCompaction(t.Context(), "ses_1", 8, 6, 1, replacement...); err != nil {
		t.Fatal(err)
	}
	if len(compactions.plan.Runs) != 4 {
		t.Fatalf("planned Runs = %d, want 4", len(compactions.plan.Runs))
	}
	wantMarks := []int{1, 1, 3, run.UnknownMessageMark}
	for index, planned := range compactions.plan.Runs {
		if !planned.Expected.Equal(compactions.runs[index]) {
			t.Fatalf("planned Run %d lost its expected CAS aggregate", index)
		}
		if got := planned.Replacement.MessageMark(); got != wantMarks[index] {
			t.Errorf("replacement mark[%d] = %d, want %d", index, got, wantMarks[index])
		}
	}
}
