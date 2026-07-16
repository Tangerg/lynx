package bootstrap

import (
	"testing"

	agentruntime "github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
)

func TestRuntimeConfigInjectsDurableIdentityPolicy(t *testing.T) {
	const buildID = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	got := RuntimeConfig(config.Config{}, &persistence.Bundle{Home: t.TempDir()}, nil, nil, nil, buildID)
	if got.Engine.BuildID != buildID {
		t.Fatalf("Engine.BuildID = %q, want %q", got.Engine.BuildID, buildID)
	}
	if got.Engine.SnapshotFailurePolicy != agentruntime.SnapshotFailureFailProcess {
		t.Fatalf("SnapshotFailurePolicy = %s, want fail_process", got.Engine.SnapshotFailurePolicy)
	}
}
