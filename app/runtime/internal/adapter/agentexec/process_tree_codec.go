package agentexec

import (
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// encodeProcessTree closes the Agent-to-App checkpoint boundary. The complete
// framework tree becomes one opaque payload, so no outer layer can acquire or
// reproduce executor topology.
func encodeProcessTree(tree core.ProcessSnapshotTree) ([]byte, error) {
	if err := tree.Validate(); err != nil {
		return nil, fmt.Errorf("agentexec: encode process tree: %w", err)
	}
	payload, err := json.Marshal(tree)
	if err != nil {
		return nil, fmt.Errorf("agentexec: encode process tree: %w", err)
	}
	return payload, nil
}

// decodeValidatedProcessTree is the only App path that interprets a validated
// opaque executor payload. The decoded root must agree with the App-owned
// aggregate identity so storage corruption cannot redirect a continuation.
func decodeValidatedProcessTree(checkpoint execution.ExecutorCheckpoint) (core.ProcessSnapshotTree, error) {
	var tree core.ProcessSnapshotTree
	if err := json.Unmarshal(checkpoint.Payload, &tree); err != nil {
		return core.ProcessSnapshotTree{}, fmt.Errorf(
			"agentexec: decode process tree %q: %w: %w",
			checkpoint.RootProcessID,
			core.ErrInvalidSnapshot,
			err,
		)
	}
	if tree.RootID != checkpoint.RootProcessID {
		return core.ProcessSnapshotTree{}, fmt.Errorf(
			"agentexec: decoded process tree root %q differs from checkpoint root %q: %w",
			tree.RootID,
			checkpoint.RootProcessID,
			core.ErrInvalidSnapshot,
		)
	}
	if err := tree.Validate(); err != nil {
		return core.ProcessSnapshotTree{}, fmt.Errorf("agentexec: decode process tree %q: %w", checkpoint.RootProcessID, err)
	}
	return tree, nil
}
