package sqlite

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

func TestStoredEnumDecodersRejectNarrowingOverflow(t *testing.T) {
	for _, stored := range []int{-1, 256} {
		if _, err := decodeRunFailureKind(stored); err == nil {
			t.Errorf("decodeRunFailureKind(%d) accepted an out-of-range value", stored)
		}
		if _, err := decodeStoredItemKind(stored); err == nil {
			t.Errorf("decodeStoredItemKind(%d) accepted an out-of-range value", stored)
		}
	}

	if got, err := decodeRunFailureKind(int(run.FailureProviderRejected)); err != nil || got != run.FailureProviderRejected {
		t.Fatalf("decodeRunFailureKind(valid) = (%d, %v)", got, err)
	}
	if got, err := decodeStoredItemKind(int(transcript.ToolCall)); err != nil || got != transcript.ToolCall {
		t.Fatalf("decodeStoredItemKind(valid) = (%d, %v)", got, err)
	}
}

func TestToolCancellationFailureKindRoundTrips(t *testing.T) {
	encoded, err := encodeToolFailureKind(tool.FailureCanceled)
	if err != nil {
		t.Fatal(err)
	}
	if encoded != "tool_canceled" {
		t.Fatalf("encoded canceled Tool failure = %q", encoded)
	}
	decoded, err := decodeToolFailureKind(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != tool.FailureCanceled {
		t.Fatalf("decoded canceled Tool failure = %v", decoded)
	}
}
