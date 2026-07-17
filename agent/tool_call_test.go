package agent_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent"
)

func TestToolCallFromContextWithoutManagedCall(t *testing.T) {
	if call, ok := agent.ToolCallFromContext(t.Context()); ok {
		t.Fatalf("ToolCallFromContext() = %+v, true; want zero value, false", call)
	}
}
