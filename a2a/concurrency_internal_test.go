package a2a

import (
	"testing"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

func TestToolConcurrencyKeyDeclaresIndependentTasks(t *testing.T) {
	client := new(a2aclient.Client)
	tool, err := newTool(toolConfig{
		Client: client,
		Card:   &sdka2a.AgentCard{Name: "Remote Agent"},
		Name:   "remote_agent",
	})
	if err != nil {
		t.Fatalf("newTool: %v", err)
	}

	key, concurrent := tool.ConcurrencyKey(`{"message":"one"}`)
	if key != "" || !concurrent {
		t.Fatalf("ConcurrencyKey() = %q, %v, want no conflict and concurrent", key, concurrent)
	}
}
