package runs

import (
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
)

func TestToolItemStartCannotDivergeFromItsDurableFact(t *testing.T) {
	arguments, err := tool.ParseArguments(`{"path":"README.md"}`)
	if err != nil {
		t.Fatalf("ParseArguments: %v", err)
	}
	item, err := transcript.NewToolCall(transcript.ItemIdentity{
		SessionID: "session-1", RunID: "run-1", ItemID: "item-1",
		OccurredAt: time.Unix(1, 0).UTC(),
	}, transcript.ToolInvocation{Name: "read_file", Arguments: arguments}, tool.SafetyClassSafe)
	if err != nil {
		t.Fatalf("NewToolCall: %v", err)
	}
	start, err := newToolItemStart(item)
	if err != nil {
		t.Fatalf("newToolItemStart: %v", err)
	}
	if err := start.validate(); err != nil {
		t.Fatalf("validate canonical start: %v", err)
	}

	start.ToolInvocation.Name = "other_tool"
	if err := start.validate(); err == nil {
		t.Fatal("validate accepted a presentation invocation different from the durable Item")
	}
}
