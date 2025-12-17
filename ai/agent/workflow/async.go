package workflow

import (
	"context"
)

var _ Node = (*Async)(nil)

type Async struct {
}

func (a *Async) Name() string {
	return "async"
}

func (a *Async) Run(ctx context.Context, input State) (State, error) {
	//TODO implement me
	panic("implement me")
}
