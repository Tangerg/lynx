package agentexec

import (
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// encodeProcessTreeState closes the Agent-to-App process-state boundary. The
// adapter serializes framework snapshots and projects only the topology App
// owns; storage receives payload bytes it must never interpret.
func encodeProcessTreeState(tree core.ProcessSnapshotTree) (execution.ProcessTreeState, error) {
	if err := tree.Validate(); err != nil {
		return execution.ProcessTreeState{}, fmt.Errorf("agentexec: encode process tree state: %w", err)
	}
	state := execution.ProcessTreeState{
		RootID:    tree.RootID,
		Processes: make([]execution.ProcessState, len(tree.Snapshots)),
	}
	for index, snapshot := range tree.Snapshots {
		payload, err := json.Marshal(snapshot)
		if err != nil {
			return execution.ProcessTreeState{}, fmt.Errorf(
				"agentexec: encode process tree state: snapshot %q: %w",
				snapshot.ID,
				err,
			)
		}
		state.Processes[index] = execution.ProcessState{
			ID:        snapshot.ID,
			ParentID:  snapshot.ParentID,
			StartedAt: snapshot.StartedAt,
			Payload:   payload,
		}
	}
	if err := state.Validate(); err != nil {
		return execution.ProcessTreeState{}, fmt.Errorf("agentexec: encode process tree state: %w", err)
	}
	return state, nil
}

// decodeProcessTreeState is the only App path that interprets the opaque
// executor payload. Envelope identity must agree with the framework value, so
// storage corruption cannot redirect one process state under another ID.
func decodeProcessTreeState(state execution.ProcessTreeState) (core.ProcessSnapshotTree, error) {
	if err := state.Validate(); err != nil {
		return core.ProcessSnapshotTree{}, fmt.Errorf("agentexec: decode process tree state: %w", err)
	}
	tree := core.ProcessSnapshotTree{
		RootID:    state.RootID,
		Snapshots: make([]core.ProcessSnapshot, len(state.Processes)),
	}
	for index, process := range state.Processes {
		var snapshot core.ProcessSnapshot
		if err := json.Unmarshal(process.Payload, &snapshot); err != nil {
			return core.ProcessSnapshotTree{}, fmt.Errorf(
				"agentexec: decode process tree state: process %q payload: %w: %w",
				process.ID,
				core.ErrInvalidSnapshot,
				err,
			)
		}
		if snapshot.ID != process.ID ||
			snapshot.ParentID != process.ParentID ||
			!snapshot.StartedAt.Equal(process.StartedAt) {
			return core.ProcessSnapshotTree{}, fmt.Errorf(
				"agentexec: decode process tree state: process %q envelope differs from payload: %w",
				process.ID,
				core.ErrInvalidSnapshot,
			)
		}
		tree.Snapshots[index] = snapshot
	}
	if err := tree.Validate(); err != nil {
		return core.ProcessSnapshotTree{}, fmt.Errorf("agentexec: decode process tree state: %w", err)
	}
	return tree, nil
}
