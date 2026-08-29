package a2a

import (
	"testing"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestToolConcurrencyKeyDeclaresIndependentTasks(t *testing.T) {
	client := new(a2aclient.Client)
	tool, err := newRemoteTool(remoteToolConfig{
		client: client,
		card:   &sdka2a.AgentCard{Name: "Remote Agent"},
		name:   "remote_agent",
	})
	if err != nil {
		t.Fatalf("newRemoteTool: %v", err)
	}

	binding, err := toolcontract.Bind(tool)
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := binding.Prepare(chat.ToolCall{
		ID: "test-call", Name: binding.Definition().Name, Arguments: `{"message":"one"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, concurrent := tool.ConcurrencyKey(invocation)
	if key != "" || !concurrent {
		t.Fatalf("ConcurrencyKey() = %q, %v, want no conflict and concurrent", key, concurrent)
	}
}
