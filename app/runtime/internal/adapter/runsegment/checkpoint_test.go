package runsegment

import "context"

type processCheckpointWriteFunc func(context.Context) error

func (write processCheckpointWriteFunc) PersistCheckpoint(ctx context.Context) error {
	return write(ctx)
}

func noopProcessCheckpoint(context.Context) error { return nil }
