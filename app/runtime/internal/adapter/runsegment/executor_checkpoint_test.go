package runsegment

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

func testRootExecutorCheckpoint() runs.ExecutorCheckpoint {
	const rootMemberID = "member_1"

	selection, err := modelref.New("anthropic", "claude")
	if err != nil {
		panic(err)
	}
	return runs.ExecutorCheckpoint{
		RootMemberID:   rootMemberID,
		Payload:        []byte("opaque root checkpoint"),
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

func (r *recordingExecutorCheckpointStore) SaveCheckpoint(
	_ context.Context,
	checkpoint runs.ExecutorCheckpoint,
) error {
	r.saved = append(r.saved, checkpoint.Clone())
	return r.saveErr
}

func (r *recordingExecutorCheckpointStore) LoadCheckpoint(
	_ context.Context,
	rootMemberID string,
) (runs.ExecutorCheckpoint, error) {
	for index := len(r.saved) - 1; index >= 0; index-- {
		if r.saved[index].RootMemberID == rootMemberID {
			return r.saved[index].Clone(), nil
		}
	}
	return runs.ExecutorCheckpoint{}, runs.ErrExecutorCheckpointNotFound
}

func (r *recordingExecutorCheckpointStore) DeleteCheckpoints(
	_ context.Context,
	_ string,
	rootIDs []string,
) error {
	r.deleted = append(r.deleted, append([]string(nil), rootIDs...))
	return r.deleteErr
}
