package workflow

import (
	"context"
)

var _ Node = (*Loop)(nil)

type Loop struct {
}

func (l *Loop) Name() string {
	return "loop"
}

func (l *Loop) Run(ctx context.Context, input State) (State, error) {
	//TODO implement me
	panic("implement me")
}
