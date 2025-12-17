package workflow

import (
	"context"
)

type Processor func(context.Context, State) (State, error)

func (p Processor) Name() string {
	return "processor"
}

func (p Processor) Run(ctx context.Context, state State) (State, error) {
	//TODO implement me
	panic("implement me")
}
