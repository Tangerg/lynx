package workflow

import (
	"context"
)

var _ Node = (*Batch)(nil)

type Batch struct {
}

func (b *Batch) Name() string {
	return "batch"
}

func (b *Batch) Run(ctx context.Context, input State) (State, error) {
	//TODO implement me
	panic("implement me")
}
