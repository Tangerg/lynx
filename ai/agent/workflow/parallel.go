package workflow

import (
	"context"
)

var _ Node = (*Parallel)(nil)

type Parallel struct {
}

func (p *Parallel) Name() string {
	return "parallel"
}

func (p *Parallel) Run(ctx context.Context, input State) (State, error) {
	//TODO implement me
	panic("implement me")
}
