package sqlite

import (
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
)

func TestItemKindRejectsUnknownPersistentIdentity(t *testing.T) {
	if kind := transcript.ItemKind("unknown"); kind.Valid() {
		t.Fatalf("ItemKind(%q) is valid", kind)
	}
	if !transcript.ToolCall.Valid() {
		t.Fatal("ToolCall is invalid")
	}
}

func TestToolCancellationFailureKindRoundTrips(t *testing.T) {
	encoded := tool.FailureCanceled.String()
	if encoded != "tool_canceled" {
		t.Fatalf("encoded canceled Tool failure = %q", encoded)
	}
	decoded := tool.FailureKind(encoded)
	if decoded != tool.FailureCanceled {
		t.Fatalf("decoded canceled Tool failure = %v", decoded)
	}
}
