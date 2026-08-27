package conversation

import (
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

func TestCompactionRebasesHistoricalWatermarks(t *testing.T) {
	replacement := []chat.Message{
		chat.NewSystemMessage("summary"),
		chat.NewUserMessage(chat.NewTextPart("recent question")),
		chat.NewAssistantMessage(chat.NewTextPart("recent answer")),
	}
	compaction, err := NewCompaction(8, 6, 1, replacement)
	if err != nil {
		t.Fatal(err)
	}
	for mark, want := range map[int]int{0: 0, 1: 1, 4: 1, 6: 1, 7: 2, 8: 3} {
		got, err := compaction.RebaseMessageMark(mark)
		if err != nil || got != want {
			t.Errorf("RebaseMessageMark(%d) = (%d, %v), want (%d, nil)", mark, got, err, want)
		}
	}
	if _, err := compaction.RebaseMessageMark(9); err == nil {
		t.Fatal("watermark beyond the pre-compaction history must fail")
	}
}

func TestContentOnlyCompactionPreservesCoordinates(t *testing.T) {
	replacement := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("trimmed")),
		chat.NewAssistantMessage(chat.NewTextPart("answer")),
	}
	compaction, err := NewCompaction(2, 0, 0, replacement)
	if err != nil {
		t.Fatal(err)
	}
	for mark := range 3 {
		got, err := compaction.RebaseMessageMark(mark)
		if err != nil || got != mark {
			t.Errorf("RebaseMessageMark(%d) = (%d, %v)", mark, got, err)
		}
	}
}
