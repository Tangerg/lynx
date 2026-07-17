package a2a

import (
	"errors"
	"testing"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"

	corechat "github.com/Tangerg/lynx/core/chat"
)

func TestNewToolValidatesPublicDefinition(t *testing.T) {
	_, err := newTool(toolConfig{
		Client: new(a2aclient.Client),
		Card:   &sdka2a.AgentCard{Name: "Remote Agent"},
		Name:   "invalid name",
	})
	if !errors.Is(err, corechat.ErrInvalidToolDefinition) {
		t.Fatalf("newTool error = %v, want chat.ErrInvalidToolDefinition", err)
	}
}
