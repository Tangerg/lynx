package workflow

import (
	"context"
)

var _ Node = (*Flow)(nil)

type Flow struct {
}

func (f *Flow) Name() string {
	return "flow"
}

func (f *Flow) Run(ctx context.Context, input State) (State, error) {
	//TODO implement me
	panic("implement me")
}
