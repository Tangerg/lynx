package runs

import "context"

type noopProcessCheckpoint struct{}

func (noopProcessCheckpoint) PersistCheckpoint(context.Context) error { return nil }
