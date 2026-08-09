package runsegment

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

func testRootExecutorCheckpoint() runs.ExecutorCheckpoint {
	const rootProcessID = "proc_1"

	selection, err := modelref.New("anthropic", "claude")
	if err != nil {
		panic(err)
	}
	return runs.ExecutorCheckpoint{
		RootMemberID:   rootProcessID,
		Payload:        []byte(`{"root":"` + rootProcessID + `"}`),
		BuildID:        "build",
		Scope:          runs.ExecutionScope{SessionID: "ses_1"},
		ModelSelection: selection,
	}
}

type recordingExecutorCheckpointStore struct {
	saved     []runs.ExecutorCheckpoint
	deleted   [][]string
	saveErr   error
	deleteErr error
}

func (store *recordingExecutorCheckpointStore) SaveCheckpoint(
	_ context.Context,
	checkpoint runs.ExecutorCheckpoint,
) error {
	store.saved = append(store.saved, checkpoint.Clone())
	return store.saveErr
}

func (store *recordingExecutorCheckpointStore) LoadCheckpoint(
	_ context.Context,
	rootMemberID string,
) (runs.ExecutorCheckpoint, error) {
	for index := len(store.saved) - 1; index >= 0; index-- {
		if store.saved[index].RootMemberID == rootMemberID {
			return store.saved[index].Clone(), nil
		}
	}
	return runs.ExecutorCheckpoint{}, runs.ErrExecutorCheckpointNotFound
}

func (store *recordingExecutorCheckpointStore) DeleteCheckpoints(
	_ context.Context,
	_ string,
	rootIDs []string,
) error {
	store.deleted = append(store.deleted, append([]string(nil), rootIDs...))
	return store.deleteErr
}
