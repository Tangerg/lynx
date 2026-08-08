package sqlite

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

func TestStoredEnumDecodersRejectNarrowingOverflow(t *testing.T) {
	for _, stored := range []int{-1, 256} {
		if _, err := decodeRunProblemKind(stored); err == nil {
			t.Errorf("decodeRunProblemKind(%d) accepted an out-of-range value", stored)
		}
		if _, err := decodeStoredItemKind(stored); err == nil {
			t.Errorf("decodeStoredItemKind(%d) accepted an out-of-range value", stored)
		}
	}

	if got, err := decodeRunProblemKind(int(transcript.ToolFailedProblem)); err != nil || got != transcript.ToolFailedProblem {
		t.Fatalf("decodeRunProblemKind(valid) = (%d, %v)", got, err)
	}
	if got, err := decodeStoredItemKind(int(transcript.ToolCall)); err != nil || got != transcript.ToolCall {
		t.Fatalf("decodeStoredItemKind(valid) = (%d, %v)", got, err)
	}
}
