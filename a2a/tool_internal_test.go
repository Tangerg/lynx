package a2a

import (
	"errors"
	"testing"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"

	corechat "github.com/Tangerg/scope/core/chat"
)

func TestNewToolValidatesPublicDefinition(t *testing.T) {
	_, err := newRemoteTool(remoteToolConfig{
		client: new(a2aclient.Client),
		card:   &sdka2a.AgentCard{Name: "Remote Agent"},
		name:   "invalid name",
	})
	if !errors.Is(err, corechat.ErrInvalidToolDefinition) {
		t.Fatalf("newRemoteTool error = %v, want chat.ErrInvalidToolDefinition", err)
	}
}
