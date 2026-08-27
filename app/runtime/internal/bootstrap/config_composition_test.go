package bootstrap

import (
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/scope/app/runtime/internal/config"
	sqlitestore "github.com/Tangerg/scope/app/runtime/internal/infra/sqlite"
)

func TestComposeConfigInjectsDurableRuntimePolicy(t *testing.T) {
	const buildID = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	agentMemory := sqlitestore.NewAgentMemoryStore(nil)
	got := ComposeConfig(config.Settings{}, &persistence.Bundle{DataDirectory: t.TempDir(), AgentMemory: agentMemory}, nil, nil, nil, buildID)
	if got.BuildID != buildID {
		t.Fatalf("BuildID = %q, want %q", got.BuildID, buildID)
	}
	if got.AgentMemoryStore != agentMemory {
		t.Fatal("agent memory was not wired to consolidation and prompt composition")
	}
}
