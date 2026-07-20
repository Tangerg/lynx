package bootstrap

import (
	"testing"

	agentruntime "github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
	sqlitestore "github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func TestRuntimeConfigInjectsDurableRuntimePolicy(t *testing.T) {
	const buildID = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	agentMemory := sqlitestore.NewAgentMemoryStore(nil)
	got := RuntimeConfig(config.Config{}, &persistence.Bundle{Home: t.TempDir(), AgentMemory: agentMemory}, nil, nil, nil, buildID)
	if got.Engine.BuildID != buildID {
		t.Fatalf("Engine.BuildID = %q, want %q", got.Engine.BuildID, buildID)
	}
	if got.Engine.SnapshotFailurePolicy != agentruntime.SnapshotFailureFailProcess {
		t.Fatalf("SnapshotFailurePolicy = %s, want fail_process", got.Engine.SnapshotFailurePolicy)
	}
	if got.AgentMemoryStore != agentMemory || got.Engine.AgentMemory != agentMemory {
		t.Fatal("agent memory was not wired to extraction and prompt composition")
	}
}
