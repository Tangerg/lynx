package agentexec

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

func TestProcessTreeStateCodecRoundTripOwnsFrameworkTranslation(t *testing.T) {
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
	state, err := encodeProcessTreeState(tree)
	if err != nil {
		t.Fatalf("encodeProcessTreeState: %v", err)
	}
	if len(state.Processes) != 2 || state.Processes[1].ParentID != "root" {
		t.Fatalf("encoded state = %+v", state)
	}
	restored, err := decodeProcessTreeState(state)
	if err != nil {
		t.Fatalf("decodeProcessTreeState: %v", err)
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

	state.Processes[1].StartedAt = state.Processes[1].StartedAt.Add(time.Second)
	if _, err := decodeProcessTreeState(state); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("decode mismatched envelope error = %v, want ErrInvalidSnapshot", err)
	}
}
