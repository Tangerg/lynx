package agentexec

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

func TestProcessTreeCodecKeepsFrameworkTopologyInsideExecutionAdapter(t *testing.T) {
	startedAt := time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)
	tree := core.ProcessSnapshotTree{
		RootID: "root",
		Snapshots: []core.ProcessSnapshot{
			{
				SchemaVersion: core.ProcessSnapshotSchemaVersion,
				ID:            "root",
				Deployment:    core.DeploymentRef{Name: "root-agent", Digest: "root-digest"},
				StartedAt:     startedAt,
				Status:        core.StatusCompleted,
			},
			{
				SchemaVersion: core.ProcessSnapshotSchemaVersion,
				ID:            "child",
				ParentID:      "root",
				Deployment:    core.DeploymentRef{Name: "child-agent", Digest: "child-digest"},
				StartedAt:     startedAt.Add(time.Second),
				Status:        core.StatusKilled,
			},
		},
	}
	payload, err := encodeProcessTree(tree)
	if err != nil {
		t.Fatalf("encodeProcessTree: %v", err)
	}
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID: tree.RootID,
		Payload:      payload,
		BuildID:      "build",
		Scope:        runs.ExecutionScope{SessionID: "session"},
	}
	if err := checkpoint.Validate(); err != nil {
		t.Fatalf("validate checkpoint: %v", err)
	}
	restored, err := decodeValidatedProcessTree(checkpoint)
	if err != nil {
		t.Fatalf("decodeValidatedProcessTree: %v", err)
	}
	if restored.RootID != tree.RootID || len(restored.Snapshots) != len(tree.Snapshots) {
		t.Fatalf("restored tree = %+v, want %+v", restored, tree)
	}
	for index := range tree.Snapshots {
		if restored.Snapshots[index].ID != tree.Snapshots[index].ID ||
			restored.Snapshots[index].ParentID != tree.Snapshots[index].ParentID ||
			!restored.Snapshots[index].StartedAt.Equal(tree.Snapshots[index].StartedAt) {
			t.Fatalf("restored snapshot[%d] = %+v, want %+v", index, restored.Snapshots[index], tree.Snapshots[index])
		}
	}

	checkpoint.RootMemberID = "another-root"
	if _, err := decodeValidatedProcessTree(checkpoint); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("decode mismatched root error = %v, want ErrInvalidSnapshot", err)
	}
}
